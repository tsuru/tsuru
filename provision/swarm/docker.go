// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/servicecommon"
)

const (
	dockerDialTimeout  = 5 * time.Second
	dockerFullTimeout  = 15 * time.Minute
	dockerTCPKeepALive = 30 * time.Second
	tsuruLabelPrefix   = "tsuru."
)

func newClient(address string, tlsConfig *tls.Config) (*docker.Client, error) {
	client, err := docker.NewClient(address)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	dialer := &net.Dialer{
		Timeout:   dockerDialTimeout,
		KeepAlive: dockerTCPKeepALive,
	}
	transport := http.Transport{
		Dial:                dialer.Dial,
		TLSHandshakeTimeout: dockerDialTimeout,
		TLSClientConfig:     tlsConfig,
		// No connection pooling so that we have reliable dial timeouts. Slower
		// but safer.
		DisableKeepAlives:   true,
		MaxIdleConnsPerHost: -1,
	}
	httpClient := &http.Client{
		Transport: &transport,
		Timeout:   dockerFullTimeout,
	}
	client.HTTPClient = httpClient
	client.Dialer = dialer
	client.TLSConfig = tlsConfig
	return client, nil
}

func joinSwarm(existingClient *clusterClient, newClient *docker.Client, addr string) error {
	swarmInfo, err := existingClient.InspectSwarm(context.TODO())
	if err != nil {
		return errors.WithStack(err)
	}
	dockerInfo, err := existingClient.Info()
	if err != nil {
		return errors.WithStack(err)
	}
	if len(dockerInfo.Swarm.RemoteManagers) == 0 {
		return errors.Errorf("no remote managers found in node %#v", dockerInfo)
	}
	addrs := make([]string, len(dockerInfo.Swarm.RemoteManagers))
	for i, peer := range dockerInfo.Swarm.RemoteManagers {
		addrs[i] = peer.Addr
	}
	host := tsuruNet.URLToHost(addr)
	opts := docker.JoinSwarmOptions{
		JoinRequest: swarm.JoinRequest{
			ListenAddr:    fmt.Sprintf("0.0.0.0:%d", swarmConfig.swarmPort),
			AdvertiseAddr: host,
			JoinToken:     swarmInfo.JoinTokens.Worker,
			RemoteAddrs:   addrs,
		},
	}
	err = newClient.JoinSwarm(opts)
	if err != nil && err != docker.ErrNodeAlreadyInSwarm {
		return errors.WithStack(err)
	}
	return nil
}

type waitResult struct {
	status int
	err    error
}

var safeAttachInspectTimeout = 20 * time.Second

func safeAttachWaitContainer(client *docker.Client, opts docker.AttachToContainerOptions) (int, error) {
	resultCh := make(chan waitResult, 1)
	go func() {
		err := client.AttachToContainer(opts)
		if err != nil {
			resultCh <- waitResult{err: err}
			return
		}
		status, err := client.WaitContainer(opts.Container)
		resultCh <- waitResult{status: status, err: err}
	}()
	for {
		select {
		case result := <-resultCh:
			return result.status, errors.Wrap(result.err, "")
		case <-time.After(safeAttachInspectTimeout):
		}
		contData, err := client.InspectContainer(opts.Container)
		if err != nil {
			return 0, errors.WithStack(err)
		}
		if !contData.State.Running {
			return contData.State.ExitCode, nil
		}
	}
}

var waitForTaskTimeout = 5 * time.Minute

func taskStatusMsg(status swarm.TaskStatus) string {
	var exitCode string
	if status.ContainerStatus != nil {
		exitCode = strconv.Itoa(status.ContainerStatus.ExitCode)
	}
	return fmt.Sprintf("state: %q, err: %q, msg: %q, container exit: %q", status.State, status.Err, status.Message, exitCode)
}

func waitForTasks(client *clusterClient, serviceID string, wantedStates ...swarm.TaskState) ([]swarm.Task, error) {
	timeout := time.After(waitForTaskTimeout)
	for {
		tasks, err := client.ListTasks(docker.ListTasksOptions{
			Filters: map[string][]string{
				"service": {serviceID},
			},
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		var inStateCount int
	eachTask:
		for _, t := range tasks {
			for _, wanted := range wantedStates {
				if t.Status.State == wanted {
					inStateCount++
					continue eachTask
				}
			}
			if t.Status.State == t.DesiredState {
				inStateCount++
			}
			if t.Status.State == swarm.TaskStateFailed || t.Status.State == swarm.TaskStateRejected {
				return nil, errors.Errorf("invalid task state for service %q: %s", serviceID, taskStatusMsg(t.Status))
			}
		}
		if len(tasks) > 0 && inStateCount == len(tasks) {
			return tasks, nil
		}
		select {
		case <-timeout:
			return nil, errors.Errorf("timeout waiting for task for service %q to be ready", serviceID)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func commitPushBuildImage(client *docker.Client, img, contID string, app provision.App) (string, error) {
	repository, tag := image.SplitImageName(img)
	_, err := client.CommitContainer(docker.CommitContainerOptions{
		Container:  contID,
		Repository: repository,
		Tag:        tag,
	})
	if err != nil {
		return "", errors.WithStack(err)
	}
	tags := []string{tag}
	if tag != "latest" {
		tags = append(tags, "latest")
		err = client.TagImage(fmt.Sprintf("%s:%s", repository, tag), docker.TagImageOptions{
			Repo:  repository,
			Tag:   "latest",
			Force: true,
		})
		if err != nil {
			return "", errors.WithStack(err)
		}
	}
	for _, tag := range tags {
		err = dockercommon.PushImage(client, repository, tag, dockercommon.RegistryAuthConfig())
		if err != nil {
			return "", err
		}
	}
	return img, nil
}

func serviceNameForApp(a provision.App, process string) string {
	return fmt.Sprintf("%s-%s", a.GetName(), process)
}

func networkNameForApp(a provision.App) string {
	return fmt.Sprintf("app-%s-overlay", a.GetName())
}

type tsuruServiceOpts struct {
	app           provision.App
	process       string
	image         string
	buildImage    string
	isDeploy      bool
	isIsolatedRun bool
	labels        *provision.LabelSet
	replicas      int
}

func extraRegisterCmds(app provision.App) string {
	host, _ := config.GetString("host")
	if !strings.HasPrefix(host, "http") {
		host = "http://" + host
	}
	if !strings.HasSuffix(host, "/") {
		host += "/"
	}
	token := app.Envs()["TSURU_APP_TOKEN"].Value
	return fmt.Sprintf(`curl -sSL -m15 -XPOST -d"hostname=$(hostname)" -o/dev/null -H"Content-Type:application/x-www-form-urlencoded" -H"Authorization:bearer %s" %sapps/%s/units/register || true`, token, host, app.GetName())
}

func serviceSpecForApp(opts tsuruServiceOpts) (*swarm.ServiceSpec, error) {
	var envs []string
	appEnvs := provision.EnvsForApp(opts.app, opts.process, opts.isDeploy)
	for _, envData := range appEnvs {
		envs = append(envs, fmt.Sprintf("%s=%s", envData.Name, envData.Value))
	}
	var cmds []string
	var err error
	var endpointSpec *swarm.EndpointSpec
	var networks []swarm.NetworkAttachmentConfig
	var healthConfig *container.HealthConfig
	port := provision.WebProcessDefaultPort()
	portInt, _ := strconv.Atoi(port)
	mounts, err := mountsForApp(opts.app)
	if err != nil {
		return nil, err
	}
	if !opts.isDeploy && !opts.isIsolatedRun {
		endpointSpec = &swarm.EndpointSpec{
			Mode: swarm.ResolutionModeVIP,
			Ports: []swarm.PortConfig{
				{TargetPort: uint32(portInt), PublishedPort: 0},
			},
		}
		networks = []swarm.NetworkAttachmentConfig{
			{Target: networkNameForApp(opts.app)},
		}
		extra := []string{extraRegisterCmds(opts.app)}
		cmds, _, err = dockercommon.LeanContainerCmdsWithExtra(opts.process, opts.image, opts.app, extra)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		var yamlData provision.TsuruYamlData
		yamlData, err = image.GetImageTsuruYamlData(opts.image)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		healthConfig = toHealthConfig(yamlData, portInt)
	}
	if opts.labels == nil {
		opts.labels, err = provision.ServiceLabels(provision.ServiceLabelsOpts{
			App:     opts.app,
			Process: opts.process,
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	provision.ExtendServiceLabels(opts.labels, provision.ServiceLabelExtendedOpts{
		IsDeploy:      opts.isDeploy,
		IsIsolatedRun: opts.isIsolatedRun,
		BuildImage:    opts.buildImage,
		Provisioner:   provisionerName,
		Prefix:        tsuruLabelPrefix,
	})
	var logDriver *swarm.Driver
	srvName := serviceNameForApp(opts.app, opts.process)
	if opts.isDeploy {
		opts.replicas = 1
		srvName = fmt.Sprintf("%s-build", srvName)
		logDriver = &swarm.Driver{
			Name: dockercommon.JsonFileLogDriver,
		}
	}
	if opts.isIsolatedRun {
		opts.replicas = 1
		srvName = fmt.Sprintf("%sisolated-run", srvName)
	}
	uReplicas := uint64(opts.replicas)
	spec := swarm.ServiceSpec{
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image:       opts.image,
				Env:         envs,
				Labels:      opts.labels.ToLabels(),
				Command:     cmds,
				Healthcheck: healthConfig,
				Mounts:      mounts,
			},
			Networks: networks,
			RestartPolicy: &swarm.RestartPolicy{
				Condition: swarm.RestartPolicyConditionAny,
			},
			Placement: &swarm.Placement{
				Constraints: []string{
					toNodePoolConstraint(opts.app.GetPool(), true),
				},
			},
			LogDriver: logDriver,
		},
		EndpointSpec: endpointSpec,
		Annotations: swarm.Annotations{
			Name:   srvName,
			Labels: opts.labels.ToLabels(),
		},
		Mode: swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{
				Replicas: &uReplicas,
			},
		},
	}
	return &spec, nil
}

func removeServiceAndLog(client *clusterClient, id string) {
	err := client.RemoveService(docker.RemoveServiceOptions{
		ID: id,
	})
	if err != nil {
		log.Errorf("error removing service: %+v", errors.WithStack(err))
	}
}

func nodeAddr(client *clusterClient, node *swarm.Node) string {
	l := provision.LabelSet{Labels: node.Spec.Annotations.Labels, Prefix: tsuruLabelPrefix}
	addr := l.NodeAddr()
	if addr != "" {
		return addr
	}
	if node.Status.Addr == "" {
		return ""
	}
	hasTLS := clusterHasTLS(client.Cluster)
	scheme := "http"
	if hasTLS {
		scheme = "https"
	}
	port, _ := config.GetInt("swarm:node-port")
	if port == 0 {
		port = 2375
		if client != nil && hasTLS {
			port = 2376
		}
	}
	return fmt.Sprintf("%s://%s:%d", scheme, node.Status.Addr, port)
}

func clientForNode(baseClient *clusterClient, nodeID string) (*docker.Client, error) {
	node, err := baseClient.InspectNode(nodeID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	tlsConfig, err := tlsConfigForCluster(baseClient.Cluster)
	if err != nil {
		return nil, err
	}
	return newClient(nodeAddr(baseClient, node), tlsConfig)
}

func runningTasksForApp(client *clusterClient, a provision.App, taskID string) ([]swarm.Task, error) {
	l, err := provision.ProcessLabels(provision.ProcessLabelsOpts{
		App:    a,
		Prefix: tsuruLabelPrefix,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	filters := map[string][]string{
		"label":         toLabelSelectors(l.ToAppSelector()),
		"desired-state": {string(swarm.TaskStateRunning)},
	}
	if taskID != "" {
		filters["id"] = []string{taskID}
	}
	tasks, err := client.ListTasks(docker.ListTasksOptions{Filters: filters})
	return tasks, errors.WithStack(err)
}

func execInTaskContainer(c *clusterClient, t *swarm.Task, stdout, stderr io.Writer, cmd string, args ...string) error {
	nodeClient, err := clientForNode(c, t.NodeID)
	if err != nil {
		return err
	}
	cmds := []string{"/bin/bash", "-lc", cmd}
	cmds = append(cmds, args...)
	execCreateOpts := docker.CreateExecOptions{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          cmds,
		Container:    taskContainerID(t),
	}
	exec, err := nodeClient.CreateExec(execCreateOpts)
	if err != nil {
		return errors.WithStack(err)
	}
	startExecOptions := docker.StartExecOptions{
		OutputStream: stdout,
		ErrorStream:  stderr,
	}
	err = nodeClient.StartExec(exec.ID, startExecOptions)
	if err != nil {
		return errors.WithStack(err)
	}
	execData, err := nodeClient.InspectExec(exec.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if execData.ExitCode != 0 {
		return fmt.Errorf("unexpected exit code: %d", execData.ExitCode)
	}
	return nil
}

func serviceSpecForNodeContainer(config *nodecontainer.NodeContainerConfig, pool string, filter servicecommon.PoolFilter) (*swarm.ServiceSpec, error) {
	var constraints []string
	if len(filter.Exclude) > 0 {
		for _, v := range filter.Exclude {
			constraints = append(constraints, toNodePoolConstraint(v, false))
		}
	} else {
		for _, v := range filter.Include {
			constraints = append(constraints, toNodePoolConstraint(v, true))
		}
	}
	var mounts []mount.Mount
	for _, b := range config.HostConfig.Binds {
		parts := strings.SplitN(b, ":", 3)
		mount := mount.Mount{Type: mount.TypeBind, Source: parts[0]}
		if len(parts) > 1 {
			mount.Target = parts[1]
		}
		if len(parts) > 2 {
			mount.ReadOnly = parts[2] == "ro"
		}
		mounts = append(mounts, mount)
	}
	var healthcheck *container.HealthConfig
	if config.Config.Healthcheck != nil {
		healthcheck = &container.HealthConfig{
			Test:     config.Config.Healthcheck.Test,
			Interval: config.Config.Healthcheck.Interval,
			Timeout:  config.Config.Healthcheck.Timeout,
			Retries:  config.Config.Healthcheck.Retries,
		}
	}
	labels := provision.NodeContainerLabels(provision.NodeContainerLabelsOpts{
		Name:         config.Name,
		CustomLabels: config.Config.Labels,
		Pool:         pool,
		Provisioner:  "swarm",
		Prefix:       tsuruLabelPrefix,
	}).ToLabels()
	service := &swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name:   nodeContainerServiceName(config.Name, pool),
			Labels: labels,
		},
		Mode: swarm.ServiceMode{Global: &swarm.GlobalService{}},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image:       config.Image(),
				Labels:      labels,
				Command:     config.Config.Entrypoint,
				Args:        config.Config.Cmd,
				Env:         config.Config.Env,
				Dir:         config.Config.WorkingDir,
				User:        config.Config.User,
				TTY:         config.Config.Tty,
				Mounts:      mounts,
				Healthcheck: healthcheck,
			},
			Placement: &swarm.Placement{Constraints: constraints},
		},
	}
	if config.HostConfig.NetworkMode != "" {
		service.TaskTemplate.Networks = []swarm.NetworkAttachmentConfig{
			{Target: config.HostConfig.NetworkMode},
		}
	}
	return service, nil
}

func upsertService(spec swarm.ServiceSpec, client *clusterClient, placementOnly bool) (bool, error) {
	currService, err := client.InspectService(spec.Name)
	if err != nil {
		if _, ok := err.(*docker.NoSuchService); !ok {
			return false, errors.WithStack(err)
		}
		opts := docker.CreateServiceOptions{ServiceSpec: spec}
		_, errCreate := client.CreateService(opts)
		if errCreate != nil {
			return false, errors.WithStack(errCreate)
		}
		return true, nil
	}
	if placementOnly {
		currService.Spec.TaskTemplate.Placement = spec.TaskTemplate.Placement
		spec = currService.Spec
	}
	opts := docker.UpdateServiceOptions{
		ServiceSpec: spec,
		Version:     currService.Version.Index,
	}
	return false, errors.WithStack(client.UpdateService(currService.ID, opts))
}

func nodeContainerServiceName(name, pool string) string {
	if pool == "" {
		return fmt.Sprintf("node-container-%s-all", name)
	}
	return fmt.Sprintf("node-container-%s-%s", name, pool)
}

func toLabelSelectors(m map[string]string) []string {
	var selectors []string
	for k, v := range m {
		selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
	}
	return selectors
}

func (p *swarmProvisioner) cleanImageInNodes(imgName string) error {
	nodes, err := p.ListNodes(nil)
	if err != nil {
		return err
	}
	multi := tsuruErrors.NewMultiError()
	for _, n := range nodes {
		nodeWrapper := n.(*swarmNodeWrapper)
		tls, err := tlsConfigForCluster(nodeWrapper.client.Cluster)
		if err != nil {
			multi.Add(err)
			continue
		}
		client, err := newClient(n.Address(), tls)
		if err != nil {
			multi.Add(err)
			continue
		}
		err = client.RemoveImage(imgName)
		if err != nil && err != docker.ErrNoSuchImage {
			multi.Add(errors.WithStack(err))
		}
	}
	return multi.ToError()
}

func toNodePoolConstraint(pool string, equal bool) string {
	operator := "=="
	if !equal {
		operator = "!="
	}
	return fmt.Sprintf("node.labels.%s%s %s %s", tsuruLabelPrefix, provision.LabelNodePool, operator, pool)
}
