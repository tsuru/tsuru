// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/engine-api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/safe"
)

const (
	dockerDialTimeout  = 5 * time.Second
	dockerFullTimeout  = time.Minute
	dockerTCPKeepALive = 30 * time.Second
)

type tsuruLabel string

func (l tsuruLabel) String() string {
	return string(l)
}

var (
	labelService           = tsuruLabel("tsuru.service")
	labelServiceDeploy     = tsuruLabel("tsuru.service.deploy")
	labelServiceBuildImage = tsuruLabel("tsuru.service.buildImage")
	labelAppName           = tsuruLabel("tsuru.app.name")
	labelAppProcess        = tsuruLabel("tsuru.app.process")
	labelAppPlatform       = tsuruLabel("tsuru.app.platform")
	labelRouterName        = tsuruLabel("tsuru.router.name")
	labelRouterType        = tsuruLabel("tsuru.router.type")
)

func newClient(address string) (*docker.Client, error) {
	client, err := docker.NewClient(address)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	dialer := &net.Dialer{
		Timeout:   dockerDialTimeout,
		KeepAlive: dockerTCPKeepALive,
	}
	transport := http.Transport{
		Dial:                dialer.Dial,
		TLSHandshakeTimeout: dockerDialTimeout,
		TLSClientConfig:     swarmConfig.tlsConfig,
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
	client.TLSConfig = swarmConfig.tlsConfig
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
		return errors.Wrap(err, "")
	}
	return nil
}

func joinSwarm(existingClient *docker.Client, newClient *docker.Client, addr string) error {
	swarmInfo, err := existingClient.InspectSwarm(nil)
	if err != nil {
		return errors.Wrap(err, "")
	}
	dockerInfo, err := existingClient.Info()
	if err != nil {
		return errors.Wrap(err, "")
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
			JoinToken:     swarmInfo.JoinTokens.Manager,
			RemoteAddrs:   addrs,
		},
	}
	err = newClient.JoinSwarm(opts)
	if err != nil {
		return errors.Wrap(err, "")
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
			return 0, errors.Wrap(err, "")
		}
		if !contData.State.Running {
			return contData.State.ExitCode, nil
		}
	}
}

var waitForTaskTimeout = 30 * time.Second

func waitForTasks(client *docker.Client, serviceID string, wantedState swarm.TaskState) ([]swarm.Task, error) {
	timeout := time.After(waitForTaskTimeout)
	for {
		tasks, err := client.ListTasks(docker.ListTasksOptions{
			Filters: map[string][]string{
				"service": {serviceID},
			},
		})
		if err != nil {
			return nil, errors.Wrap(err, "")
		}
		var inStateCount int
		for _, t := range tasks {
			if t.Status.State == wantedState || t.Status.State == t.DesiredState {
				inStateCount++
			}
			if t.Status.State == swarm.TaskStateFailed || t.Status.State == swarm.TaskStateRejected {
				return nil, errors.Errorf("invalid task state for service %q: %s", serviceID, t.Status.State)
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
		return "", errors.Wrap(err, "")
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
		pushOpts := docker.PushImageOptions{Name: repo, Tag: tag, OutputStream: &buf, InactivityTimeout: tsuruNet.StreamInactivityTimeout}
		err = client.PushImage(pushOpts, registryAuthConfig())
		if err != nil {
			return errors.Wrap(err, "")
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

type tsuruServiceOpts struct {
	app        provision.App
	process    string
	image      string
	buildImage string
	baseSpec   *swarm.ServiceSpec
	isDeploy   bool
}

func serviceSpecForApp(opts tsuruServiceOpts) (*swarm.ServiceSpec, error) {
	var envs []string
	for _, envData := range opts.app.Envs() {
		envs = append(envs, fmt.Sprintf("%s=%s", envData.Name, envData.Value))
	}
	host, _ := config.GetString("host")
	envs = append(envs, fmt.Sprintf("%s=%s", "TSURU_HOST", host))
	if !opts.isDeploy {
		envs = append(envs, []string{
			fmt.Sprintf("%s=%s", "port", "8888"),
			fmt.Sprintf("%s=%s", "PORT", "8888"),
		}...)
	}
	var unitCount uint64 = 1
	if opts.baseSpec != nil {
		unitCount = *opts.baseSpec.Mode.Replicated.Replicas
	}
	routerName, err := opts.app.GetRouter()
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	routerType, _, err := router.Type(routerName)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	srvName := serviceNameForApp(opts.app, opts.process)
	if opts.isDeploy {
		srvName = fmt.Sprintf("%s-build", srvName)
	}
	labels := map[string]string{
		labelService.String():           strconv.FormatBool(true),
		labelServiceDeploy.String():     strconv.FormatBool(opts.isDeploy),
		labelServiceBuildImage.String(): opts.buildImage,
		labelAppName.String():           opts.app.GetName(),
		labelAppProcess.String():        opts.process,
		labelAppPlatform.String():       opts.app.GetPlatform(),
		labelRouterName.String():        routerName,
		labelRouterType.String():        routerType,
	}
	spec := swarm.ServiceSpec{
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: swarm.ContainerSpec{
				Image:  opts.image,
				Env:    envs,
				Labels: labels,
			},
			RestartPolicy: &swarm.RestartPolicy{
				Condition: swarm.RestartPolicyConditionAny,
			},
			Placement: &swarm.Placement{
				Constraints: []string{
					fmt.Sprintf("node.labels.pool == %s", opts.app.GetPool()),
				},
			},
		},
		EndpointSpec: &swarm.EndpointSpec{
			Mode: swarm.ResolutionModeVIP,
			Ports: []swarm.PortConfig{
				{TargetPort: 8888, PublishedPort: 0},
			},
		},
		Annotations: swarm.Annotations{
			Name:   srvName,
			Labels: labels,
		},
		Mode: swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{
				Replicas: &unitCount,
			},
		},
	}
	return &spec, nil
}

func deployCmds(app provision.App, archiveURL string) []string {
	deployCmd, err := config.GetString("docker:deploy-cmd")
	if err != nil {
		deployCmd = "/var/lib/tsuru/deploy"
	}
	cmds := append([]string{deployCmd}, "archive", archiveURL)
	host, _ := config.GetString("host")
	appEnvs := app.Envs()
	var token string
	if tokenEnv, ok := appEnvs["TSURU_APP_TOKEN"]; ok {
		token = tokenEnv.Value
	}
	unitAgentCmds := []string{"tsuru_unit_agent", host, token, app.GetName(), `"` + strings.Join(cmds, " ") + `"`, "deploy"}
	finalCmd := strings.Join(unitAgentCmds, " ")
	return []string{"/bin/bash", "-lc", finalCmd}
}

func removeServiceAndLog(client *docker.Client, id string) {
	err := client.RemoveService(docker.RemoveServiceOptions{
		ID: id,
	})
	if err != nil {
		log.Errorf("error removing service: %+v", errors.Wrap(err, ""))
	}
}

func clientForNode(baseClient *docker.Client, nodeID string) (*docker.Client, error) {
	node, err := baseClient.InspectNode(nodeID)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	return newClient(node.Spec.Annotations.Labels[labelDockerAddr])
}
