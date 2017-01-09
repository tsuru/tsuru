// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
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
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/safe"
)

const (
	dockerDialTimeout  = 5 * time.Second
	dockerFullTimeout  = time.Minute
	dockerTCPKeepALive = 30 * time.Second
	maxSwarmManagers   = 7
)

type tsuruLabel string

func (l tsuruLabel) String() string {
	return string(l)
}

var (
	labelService            = tsuruLabel("tsuru.service")
	labelServiceDeploy      = tsuruLabel("tsuru.service.deploy")
	labelServiceIsolatedRun = tsuruLabel("tsuru.service.isolated.run")
	labelServiceBuildImage  = tsuruLabel("tsuru.service.buildImage")
	labelServiceRestart     = tsuruLabel("tsuru.service.restart")
	labelAppName            = tsuruLabel("tsuru.app.name")
	labelAppProcess         = tsuruLabel("tsuru.app.process")
	labelProcessReplicas    = tsuruLabel("tsuru.app.process.replicas")
	labelAppPlatform        = tsuruLabel("tsuru.app.platform")
	labelRouterName         = tsuruLabel("tsuru.router.name")
	labelRouterType         = tsuruLabel("tsuru.router.type")
	labelNodeContainer      = tsuruLabel("tsuru.nodecontainer")
	labelNodeContainerName  = tsuruLabel("tsuru.nodecontainer.name")
	labelPoolName           = tsuruLabel("tsuru.node.pool")
	labelProvisionerName    = tsuruLabel("tsuru.node.provisioner")
)

func newClient(address string) (*docker.Client, error) {
	tlsConfig, err := getNodeCredentials(address)
	if err != nil {
		return nil, err
	}
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

func initSwarm(client *docker.Client, addr string) error {
	host := tsuruNet.URLToHost(addr)
	_, err := client.InitSwarm(docker.InitSwarmOptions{
		InitRequest: swarm.InitRequest{
			ListenAddr:    fmt.Sprintf("0.0.0.0:%d", swarmConfig.swarmPort),
			AdvertiseAddr: host,
		},
	})
	if err != nil && errors.Cause(err) != docker.ErrNodeAlreadyInSwarm {
		return errors.WithStack(err)
	}
	return nil
}

func joinSwarm(existingClient *docker.Client, newClient *docker.Client, addr string) error {
	swarmInfo, err := existingClient.InspectSwarm(nil)
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
	return redistributeManagers(existingClient)
}

func redistributeManagers(cli *docker.Client) error {
	// TODO(cezarsa): distribute managers across nodes with different metadata
	// (use splitMetadata from node autoscale after it's been moved from
	// provision/docker)
	nodes, err := listValidNodes(cli)
	if err != nil {
		return err
	}
	total := len(nodes)
	if total > maxSwarmManagers {
		total = maxSwarmManagers
	}
	for i := 0; i < total; i++ {
		n := &nodes[i]
		if n.Spec.Role == swarm.NodeRoleManager {
			continue
		}
		n.Spec.Role = swarm.NodeRoleManager
		err = cli.UpdateNode(n.ID, docker.UpdateNodeOptions{
			NodeSpec: n.Spec,
			Version:  n.Version.Index,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func listValidNodes(cli *docker.Client) ([]swarm.Node, error) {
	nodes, err := cli.ListNodes(docker.ListNodesOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for i := 0; i < len(nodes); i++ {
		if _, ok := nodes[i].Spec.Annotations.Labels[labelNodeDockerAddr.String()]; !ok {
			nodes[i] = nodes[len(nodes)-1]
			nodes = nodes[:len(nodes)-1]
			i--
		}
	}
	return nodes, nil
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
	return fmt.Sprintf("state: %q, err: %q, msg: %q, container exit: %d", status.State, status.Err, status.Message, status.ContainerStatus.ExitCode)
}

func waitForTasks(client *docker.Client, serviceID string, wantedState swarm.TaskState) ([]swarm.Task, error) {
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
		for _, t := range tasks {
			if t.Status.State == wantedState || t.Status.State == t.DesiredState {
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
	parts := strings.Split(img, ":")
	repository := strings.Join(parts[:len(parts)-1], ":")
	tag := parts[len(parts)-1]
	_, err := client.CommitContainer(docker.CommitContainerOptions{
		Container:  contID,
		Repository: repository,
		Tag:        tag,
	})
	if err != nil {
		return "", errors.WithStack(err)
	}
	err = pushImage(client, repository, tag)
	if err != nil {
		return "", err
	}
	return img, nil
}

func pushImage(client *docker.Client, repo, tag string) error {
	if _, err := config.GetString("docker:registry"); err == nil {
		var buf safe.Buffer
		pushOpts := docker.PushImageOptions{Name: repo,
			Tag:               tag,
			OutputStream:      &buf,
			InactivityTimeout: tsuruNet.StreamInactivityTimeout,
			RawJSONStream:     true,
		}
		err = client.PushImage(pushOpts, registryAuthConfig())
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func registryAuthConfig() docker.AuthConfiguration {
	var authConfig docker.AuthConfiguration
	authConfig.Email, _ = config.GetString("docker:registry-auth:email")
	authConfig.Username, _ = config.GetString("docker:registry-auth:username")
	authConfig.Password, _ = config.GetString("docker:registry-auth:password")
	authConfig.ServerAddress, _ = config.GetString("docker:registry")
	return authConfig
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
	baseSpec      *swarm.ServiceSpec
	isDeploy      bool
	isIsolatedRun bool
	processState  processState
	constraints   []string
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
	return fmt.Sprintf(`curl -fsSL -m15 -XPOST -d"hostname=$(hostname)" -o/dev/null -H"Content-Type:application/x-www-form-urlencoded" -H"Authorization:bearer %s" %sapps/%s/units/register`, token, host, app.GetName())
}

func serviceSpecForApp(opts tsuruServiceOpts) (*swarm.ServiceSpec, error) {
	var envs []string
	for _, envData := range opts.app.Envs() {
		envs = append(envs, fmt.Sprintf("%s=%s", envData.Name, envData.Value))
	}
	host, _ := config.GetString("host")
	envs = append(envs, fmt.Sprintf("%s=%s", "TSURU_HOST", host))
	var cmds []string
	var err error
	var endpointSpec *swarm.EndpointSpec
	var networks []swarm.NetworkAttachmentConfig
	var healthConfig *container.HealthConfig
	port := dockercommon.WebProcessDefaultPort()
	portInt, _ := strconv.Atoi(port)
	if !opts.isDeploy && !opts.isIsolatedRun {
		envs = append(envs, []string{
			fmt.Sprintf("%s=%s", "port", port),
			fmt.Sprintf("%s=%s", "PORT", port),
		}...)
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
		healthConfig = toHealthConfig(yamlData.Healthcheck, portInt)
	}
	restartCount := 0
	replicas := 0
	if opts.baseSpec != nil {
		replicas, err = strconv.Atoi(opts.baseSpec.Labels[labelProcessReplicas.String()])
		if err != nil && opts.baseSpec.Mode.Replicated != nil {
			replicas = int(*opts.baseSpec.Mode.Replicated.Replicas)
		}
		restartCount, _ = strconv.Atoi(opts.baseSpec.Labels[labelServiceRestart.String()])
	}
	if opts.processState.increment != 0 {
		replicas += opts.processState.increment
		if replicas < 0 {
			return nil, errors.New("cannot have less than 0 units")
		}
	} else if replicas == 0 && opts.processState.start {
		replicas = 1
	}
	routerName, err := opts.app.GetRouter()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	routerType, _, err := router.Type(routerName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	srvName := serviceNameForApp(opts.app, opts.process)
	if opts.isDeploy {
		replicas = 1
		srvName = fmt.Sprintf("%s-build", srvName)
	}
	if opts.isIsolatedRun {
		replicas = 1
		srvName = fmt.Sprintf("%sisolated-run", srvName)
	}
	uReplicas := uint64(replicas)
	if opts.processState.stop {
		uReplicas = 0
	}
	if opts.processState.restart {
		restartCount++
	}
	labels := map[string]string{
		labelService.String():            strconv.FormatBool(true),
		labelServiceDeploy.String():      strconv.FormatBool(opts.isDeploy),
		labelServiceIsolatedRun.String(): strconv.FormatBool(opts.isIsolatedRun),
		labelServiceBuildImage.String():  opts.buildImage,
		labelAppName.String():            opts.app.GetName(),
		labelAppProcess.String():         opts.process,
		labelAppPlatform.String():        opts.app.GetPlatform(),
		labelRouterName.String():         routerName,
		labelRouterType.String():         routerType,
		labelProcessReplicas.String():    strconv.Itoa(replicas),
		labelServiceRestart.String():     strconv.Itoa(restartCount),
		labelPoolName.String():           opts.app.GetPool(),
		labelProvisionerName.String():    "swarm",
	}
	user, err := config.GetString("docker:user")
	if err != nil {
		user, _ = config.GetString("docker:ssh:user")
	}
	opts.constraints = append(opts.constraints, fmt.Sprintf("node.labels.%s == %s", labelNodePoolName, opts.app.GetPool()))
	spec := swarm.ServiceSpec{
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: swarm.ContainerSpec{
				Image:       opts.image,
				Env:         envs,
				Labels:      labels,
				Command:     cmds,
				User:        user,
				Healthcheck: healthConfig,
			},
			Networks: networks,
			RestartPolicy: &swarm.RestartPolicy{
				Condition: swarm.RestartPolicyConditionAny,
			},
			Placement: &swarm.Placement{
				Constraints: opts.constraints,
			},
		},
		Networks:     networks,
		EndpointSpec: endpointSpec,
		Annotations: swarm.Annotations{
			Name:   srvName,
			Labels: labels,
		},
		Mode: swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{
				Replicas: &uReplicas,
			},
		},
	}
	return &spec, nil
}

func removeServiceAndLog(client *docker.Client, id string) {
	err := client.RemoveService(docker.RemoveServiceOptions{
		ID: id,
	})
	if err != nil {
		log.Errorf("error removing service: %+v", errors.WithStack(err))
	}
}

func clientForNode(baseClient *docker.Client, nodeID string) (*docker.Client, error) {
	node, err := baseClient.InspectNode(nodeID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return newClient(node.Spec.Annotations.Labels[labelNodeDockerAddr.String()])
}

func runningTasksForApp(client *docker.Client, a provision.App, taskID string) ([]swarm.Task, error) {
	filters := map[string][]string{
		"label":         {fmt.Sprintf("%s=%s", labelAppName, a.GetName())},
		"desired-state": {string(swarm.TaskStateRunning)},
	}
	if taskID != "" {
		filters["id"] = []string{taskID}
	}
	tasks, err := client.ListTasks(docker.ListTasksOptions{Filters: filters})
	return tasks, errors.WithStack(err)
}

func execInTaskContainer(c *docker.Client, t *swarm.Task, stdout, stderr io.Writer, cmd string, args ...string) error {
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
		Container:    t.Status.ContainerStatus.ContainerID,
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

func serviceSpecForNodeContainer(name, pool string) (*swarm.ServiceSpec, error) {
	config, err := nodecontainer.LoadNodeContainer(pool, name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var constraints []string
	if pool == "" {
		otherConfigs, err := nodecontainer.LoadNodeContainersForPools(name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for p := range otherConfigs {
			if p != "" {
				constraints = append(constraints, fmt.Sprintf("node.labels.%s != %s", labelNodePoolName, p))
			}
		}
	} else {
		constraints = []string{fmt.Sprintf("node.labels.%s == %s", labelNodePoolName, pool)}
	}
	var mounts []mount.Mount
	for _, b := range config.HostConfig.Binds {
		parts := strings.SplitN(b, ":", 3)
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   parts[0],
			Target:   parts[1],
			ReadOnly: parts[2] == "ro",
		})
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
	labels := config.Config.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[labelNodeContainer.String()] = strconv.FormatBool(true)
	labels[labelNodeContainerName.String()] = name
	labels[labelPoolName.String()] = pool
	labels[labelProvisionerName.String()] = "swarm"
	service := &swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name:   nodeContainerServiceName(name, pool),
			Labels: labels,
		},
		Mode: swarm.ServiceMode{Global: &swarm.GlobalService{}},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: swarm.ContainerSpec{
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
	return service, nil
}

func upsertService(spec *swarm.ServiceSpec, client *docker.Client) (error, bool) {
	currService, err := client.InspectService(spec.Name)
	if err != nil {
		if _, ok := err.(*docker.NoSuchService); !ok {
			return errors.WithStack(err), false
		}
		opts := docker.CreateServiceOptions{ServiceSpec: *spec}
		_, errCreate := client.CreateService(opts)
		if errCreate != nil {
			return errors.WithStack(errCreate), false
		}
		return nil, true
	}
	opts := docker.UpdateServiceOptions{
		ServiceSpec: *spec,
		Version:     currService.Version.Index,
	}
	return errors.WithStack(client.UpdateService(currService.ID, opts)), false
}

func nodeContainerServiceName(name, pool string) string {
	if pool == "" {
		return fmt.Sprintf("node-container-%s-all", name)
	}
	return fmt.Sprintf("node-container-%s-%s", name, pool)
}
