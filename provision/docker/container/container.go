// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"context"
	"crypto"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

type SchedulerOpts struct {
	AppName       string
	ProcessName   string
	UpdateName    bool
	FilterNodes   []string
	ActionLimiter provision.ActionLimiter
	LimiterDone   func()
}

type SchedulerError struct {
	Base error
}

func (e *SchedulerError) Error() string {
	return fmt.Sprintf("error in scheduler: %s", e.Base)
}

type StartError struct {
	Base error
}

func (e *StartError) Error() string {
	return fmt.Sprintf("error in start container: %v", e.Base)
}

type Container struct {
	types.Container `bson:",inline"`
}

type ContainerState string

type ContainerCtxKey struct{}

var (
	ContainerStateRemoved   = ContainerState("removed")
	ContainerStateNewStatus = ContainerState("status")
	ContainerStateImageSet  = ContainerState("image")
)

type ContainerStateClient interface {
	SetContainerState(*Container, ContainerState) error
}

const (
	maxStartRetries = 4
)

func RunPipelineWithRetry(pipe *action.Pipeline, args interface{}) error {
	retryCount := maxStartRetries
	multi := tsuruErrors.NewMultiError()
	var err error
	for ; retryCount >= 0; retryCount-- {
		err = pipe.Execute(args)
		if err == nil {
			break
		}
		multi.Add(err)
		_, isStartError := errors.Cause(err).(*StartError)
		if !isStartError {
			break
		}
	}
	if err != nil {
		return multi.ToError()
	}
	return nil
}

func (c *Container) ShortID() string {
	if len(c.ID) > 10 {
		return c.ID[:10]
	}
	return c.ID
}

func (c *Container) Available() bool {
	return c.Status == provision.StatusStarted.String() ||
		c.Status == provision.StatusStarting.String()
}

func (c *Container) Address() *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%s", c.HostAddr, c.HostPort),
	}
}

type CreateArgs struct {
	ImageID          string
	Commands         []string
	App              provision.App
	Client           provision.BuilderDockerClient
	DestinationHosts []string
	ProcessName      string
	Deploy           bool
	Building         bool
	Event            *event.Event
}

func (c *Container) Create(args *CreateArgs) error {
	if len(args.DestinationHosts) > 0 {
		c.HostAddr = args.DestinationHosts[0]
	}
	var exposedPorts map[docker.Port]struct{}
	if !args.Deploy {
		if c.ExposedPort == "" {
			c.ExposedPort = provision.WebProcessDefaultPort() + "/tcp"
		}
		exposedPorts = map[docker.Port]struct{}{
			docker.Port(c.ExposedPort): {},
		}
	}
	var user string
	if args.Building {
		user, _ = dockercommon.UserForContainer()
	}
	hostConf, err := c.hostConfig(args.App, args.Deploy)
	if err != nil {
		return err
	}
	labelSet, err := provision.ProcessLabels(provision.ProcessLabelsOpts{
		App:         args.App,
		Process:     c.ProcessName,
		Provisioner: "docker",
		IsDeploy:    args.Deploy,
	})
	if err != nil {
		return err
	}
	conf := docker.Config{
		Image:        args.ImageID,
		Cmd:          args.Commands,
		Entrypoint:   []string{},
		ExposedPorts: exposedPorts,
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
		Memory:       hostConf.Memory,
		MemorySwap:   hostConf.MemorySwap,
		CPUShares:    hostConf.CPUShares,
		SecurityOpts: hostConf.SecurityOpt,
		User:         user,
		Labels:       labelSet.ToLabels(),
	}
	c.addEnvsToConfig(args, strings.TrimSuffix(c.ExposedPort, "/tcp"), &conf)
	opts := docker.CreateContainerOptions{Name: c.Name, Config: &conf, HostConfig: hostConf}
	ctx := context.WithValue(context.Background(), ContainerCtxKey{}, c)
	if args.Event != nil {
		var cancel context.CancelFunc
		ctx, cancel = args.Event.CancelableContext(ctx)
		defer cancel()
	}
	opts.Context = ctx
	cont, hostAddr, err := args.Client.PullAndCreateContainer(opts, nil)
	if err != nil {
		log.Errorf("error on creating container in docker %s - %s", c.AppName, err)
		return err
	}
	c.Name = cont.Name
	c.ID = cont.ID
	c.HostAddr = hostAddr
	return nil
}

func (c *Container) addEnvsToConfig(args *CreateArgs, port string, cfg *docker.Config) {
	envs := provision.EnvsForApp(args.App, c.ProcessName, args.Deploy)
	for _, envData := range envs {
		cfg.Env = append(cfg.Env, fmt.Sprintf("%s=%s", envData.Name, envData.Value))
	}
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	if sharedMount != "" && sharedBasedir != "" {
		cfg.Volumes = map[string]struct{}{
			sharedMount: {},
		}
		cfg.Env = append(cfg.Env, fmt.Sprintf("TSURU_SHAREDFS_MOUNTPOINT=%s", sharedMount))
	}
}

type NetworkInfo struct {
	HTTPHostPort string
	IP           string
}

func (c *Container) NetworkInfo(client provision.BuilderDockerClient) (NetworkInfo, error) {
	var netInfo NetworkInfo
	dockerContainer, err := client.InspectContainer(c.ID)
	if err != nil {
		return netInfo, err
	}
	if dockerContainer.NetworkSettings != nil {
		netInfo.IP = dockerContainer.NetworkSettings.IPAddress
		httpPort := docker.Port(c.ExposedPort)
		for _, port := range dockerContainer.NetworkSettings.Ports[httpPort] {
			if port.HostPort != "" && port.HostIP != "" {
				netInfo.HTTPHostPort = port.HostPort
				break
			}
		}
	}
	return netInfo, err
}

func (c *Container) ExpectedStatus() provision.Status {
	if c.StatusBeforeError != "" {
		return provision.Status(c.StatusBeforeError)
	}
	return provision.Status(c.Status)
}

func (c *Container) SetStatus(client provision.BuilderDockerClient, status provision.Status, triggerCallback bool) error {
	c.Status = status.String()
	c.LastStatusUpdate = time.Now().In(time.UTC)
	if c.Status != provision.StatusError.String() {
		c.StatusBeforeError = c.Status
	}
	if c.Status == provision.StatusStarted.String() ||
		c.Status == provision.StatusStarting.String() ||
		c.Status == provision.StatusStopped.String() {
		c.LastSuccessStatusUpdate = c.LastStatusUpdate
	}
	if !triggerCallback {
		return nil
	}
	return c.setState(client, ContainerStateNewStatus)
}

func (c *Container) setState(client provision.BuilderDockerClient, s ContainerState) error {
	if stateCli, ok := client.(ContainerStateClient); ok {
		return stateCli.SetContainerState(c, s)
	}
	return nil
}

func (c *Container) SetImage(client provision.BuilderDockerClient, imageID string) error {
	c.Image = imageID
	return c.setState(client, ContainerStateImageSet)
}

func (c *Container) Remove(client provision.BuilderDockerClient, limiter provision.ActionLimiter) error {
	log.Debugf("Removing container %s from docker", c.ID)
	err := c.Stop(client, limiter)
	if err != nil {
		log.Errorf("error on stop unit %s - %s", c.ID, err)
	}
	done := limiter.Start(c.HostAddr)
	err = client.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
	done()
	if err != nil {
		log.Errorf("Failed to remove container from docker: %s", err)
	}
	log.Debugf("Removing container %s from database", c.ID)
	err = c.setState(client, ContainerStateRemoved)
	if err != nil {
		log.Errorf("Failed to set new container state: %s", err)
	}
	return nil
}

type Pty struct {
	Width  int
	Height int
	Term   string
}

func (c *Container) Exec(client provision.BuilderDockerClient, stdin io.Reader, stdout, stderr io.Writer, pty Pty, cmds ...string) error {
	execClient, ok := client.(provision.ExecDockerClient)
	if !ok {
		return errors.Errorf("exec is not supported on client %T", client)
	}
	execCreateOpts := docker.CreateExecOptions{
		AttachStdin:  stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmds,
		Container:    c.ID,
		Tty:          stdin != nil,
	}
	exec, err := execClient.CreateExec(execCreateOpts)
	if err != nil {
		return err
	}
	successCh := make(chan struct{})
	startExecOptions := docker.StartExecOptions{
		InputStream:  stdin,
		OutputStream: stdout,
		ErrorStream:  stderr,
		Tty:          stdin != nil,
		RawTerminal:  stdin != nil,
		Success:      successCh,
	}
	errs := make(chan error, 1)
	go func() {
		errs <- execClient.StartExec(exec.ID, startExecOptions)
	}()
	<-successCh
	close(successCh)
	if pty.Height != 0 && pty.Width != 0 {
		execClient.ResizeExecTTY(exec.ID, pty.Height, pty.Width)
	}
	err = <-errs
	if err != nil {
		return err
	}
	execData, err := execClient.InspectExec(exec.ID)
	if err != nil {
		return err
	}
	if execData.ExitCode != 0 {
		return &execErr{code: execData.ExitCode}
	}
	return nil
}

type execErr struct {
	code int
}

func (e *execErr) Error() string {
	return fmt.Sprintf("unexpected exit code: %d", e.code)
}

// Commits commits the container, creating an image in Docker. It then returns
// the image identifier for usage in future container creation.
func (c *Container) Commit(client provision.BuilderDockerClient, limiter provision.ActionLimiter, writer io.Writer, isDeploy bool) (string, error) {
	log.Debugf("committing container %s", c.ID)
	repository, tag := image.SplitImageName(c.BuildingImage)
	opts := docker.CommitContainerOptions{Container: c.ID, Repository: repository, Tag: tag}
	done := limiter.Start(c.HostAddr)
	image, err := client.CommitContainer(opts)
	done()
	if err != nil {
		return "", log.WrapError(errors.Wrapf(err, "error in commit container %s", c.ID))
	}
	tags := []string{tag}
	if isDeploy && tag != "latest" {
		tags = append(tags, "latest")
		err = client.TagImage(fmt.Sprintf("%s:%s", repository, tag), docker.TagImageOptions{
			Repo:  repository,
			Tag:   "latest",
			Force: true,
		})
		if err != nil {
			return "", log.WrapError(errors.Wrapf(err, "error in tag container %s", c.ID))
		}
	}
	imgHistory, err := client.ImageHistory(c.BuildingImage)
	imgSize := ""
	if err == nil && len(imgHistory) > 0 {
		fullSize := imgHistory[0].Size
		if len(imgHistory) > 1 && strings.Contains(imgHistory[1].CreatedBy, "tail -f /dev/null") {
			fullSize += imgHistory[1].Size
		}
		imgSize = fmt.Sprintf("(%.02fMB)", float64(fullSize)/1024/1024)
	}
	fmt.Fprintf(writer, " ---> Sending image to repository %s\n", imgSize)
	log.Debugf("image %s generated from container %s", image.ID, c.ID)
	for _, tag := range tags {
		maxTry, _ := config.GetInt("docker:registry-max-try")
		if maxTry <= 0 {
			maxTry = 3
		}
		for i := 0; i < maxTry; i++ {
			err = dockercommon.PushImage(client, repository, tag, dockercommon.RegistryAuthConfig(repository))
			if err != nil {
				fmt.Fprintf(writer, "Could not send image, trying again. Original error: %s\n", err.Error())
				log.Errorf("error in push image %s: %s", c.BuildingImage, err)
				time.Sleep(time.Second)
				continue
			}
			break
		}
		if err != nil {
			return "", log.WrapError(errors.Wrapf(err, "error in push image %s", c.BuildingImage))
		}
	}
	return c.BuildingImage, nil
}

func (c *Container) Sleep(client provision.BuilderDockerClient, limiter provision.ActionLimiter) error {
	if c.Status != provision.StatusStarted.String() && c.Status != provision.StatusStarting.String() {
		return errors.Errorf("container %s is not starting or started", c.ID)
	}
	done := limiter.Start(c.HostAddr)
	err := client.StopContainer(c.ID, 10)
	done()
	if err != nil {
		log.Errorf("error on stop container %s: %s", c.ID, err)
	}
	return c.SetStatus(client, provision.StatusAsleep, true)
}

func (c *Container) Stop(client provision.BuilderDockerClient, limiter provision.ActionLimiter) error {
	if c.Status == provision.StatusStopped.String() {
		return nil
	}
	done := limiter.Start(c.HostAddr)
	err := client.StopContainer(c.ID, 10)
	done()
	if err != nil {
		log.Errorf("error on stop container %s: %s", c.ID, err)
	}
	c.SetStatus(client, provision.StatusStopped, true)
	return nil
}

type StartArgs struct {
	Client  provision.BuilderDockerClient
	Limiter provision.ActionLimiter
	App     provision.App
	Deploy  bool
}

func (c *Container) hostConfig(app provision.App, isDeploy bool) (*docker.HostConfig, error) {
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedIsolation, _ := config.GetBool("docker:sharedfs:app-isolation")
	sharedSalt, _ := config.GetString("docker:sharedfs:salt")
	hostConfig := docker.HostConfig{
		CPUShares: int64(app.GetCpuShare()),
	}

	if !isDeploy {
		hostConfig.Memory = app.GetMemory()
		hostConfig.MemorySwap = app.GetMemory() + app.GetSwap()
		hostConfig.RestartPolicy = docker.AlwaysRestart()
		hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
			docker.Port(c.ExposedPort): {{HostIP: "", HostPort: ""}},
		}
		pool := app.GetPool()
		driver, opts, logErr := LogOpts(pool)
		if logErr != nil {
			return nil, logErr
		}
		hostConfig.LogConfig = docker.LogConfig{
			Type:   driver,
			Config: opts,
		}
	} else {
		hostConfig.OomScoreAdj = 1000
		hostConfig.LogConfig = docker.LogConfig{
			Type: dockercommon.JsonFileLogDriver,
		}
	}

	hostConfig.SecurityOpt, _ = config.GetList("docker:security-opts")
	if sharedBasedir != "" && sharedMount != "" {
		if sharedIsolation {
			var appHostDir string
			if sharedSalt != "" {
				h := crypto.SHA1.New()
				io.WriteString(h, sharedSalt+c.AppName)
				appHostDir = fmt.Sprintf("%x", h.Sum(nil))
			} else {
				appHostDir = c.AppName
			}
			hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s/%s:%s:rw", sharedBasedir, appHostDir, sharedMount))
		} else {
			hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s:%s:rw", sharedBasedir, sharedMount))
		}
	}

	pidsLimit, _ := config.GetInt("docker:pids-limit")
	if pidsLimit > 0 {
		hostConfig.PidsLimit = int64(pidsLimit)
	}

	return &hostConfig, nil
}

func (c *Container) Start(args *StartArgs) error {
	done := args.Limiter.Start(c.HostAddr)
	err := args.Client.StartContainer(c.ID, nil)
	done()
	if err != nil {
		return &StartError{Base: err}
	}
	initialStatus := provision.StatusStarting
	if args.Deploy {
		initialStatus = provision.StatusBuilding
	}
	return c.SetStatus(args.Client, initialStatus, false)
}

func (c *Container) Logs(client provision.BuilderDockerClient, w io.Writer) (int, error) {
	container, err := client.InspectContainer(c.ID)
	if err != nil {
		return 0, err
	}
	opts := docker.AttachToContainerOptions{
		Container:    c.ID,
		Logs:         true,
		Stdout:       true,
		Stderr:       true,
		OutputStream: w,
		ErrorStream:  w,
		RawTerminal:  container.Config.Tty,
		Stream:       true,
	}
	return SafeAttachWaitContainer(client, opts)
}

func (c *Container) AsUnit(a provision.App) provision.Unit {
	status := provision.Status(c.Status)
	if c.Status == "" {
		status = provision.StatusBuilding
	}
	cType := c.Type
	if cType == "" {
		cType = a.GetPlatform()
	}
	return provision.Unit{
		ID:          c.ID,
		Name:        c.Name,
		AppName:     a.GetName(),
		Type:        cType,
		IP:          c.HostAddr,
		Status:      status,
		ProcessName: c.ProcessName,
		Address:     c.Address(),
	}
}

func (c *Container) ValidAddr() bool {
	return c.HostAddr != "" && c.HostPort != "" && c.HostPort != "0"
}

type waitResult struct {
	status int
	err    error
}

var safeAttachInspectTimeout = 20 * time.Second

func SafeAttachWaitContainer(client provision.BuilderDockerClient, opts docker.AttachToContainerOptions) (int, error) {
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
			return result.status, result.err
		case <-time.After(safeAttachInspectTimeout):
		}
		contData, err := client.InspectContainer(opts.Container)
		if err != nil {
			return 0, err
		}
		if !contData.State.Running {
			return contData.State.ExitCode, nil
		}
	}
}
