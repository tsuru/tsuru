// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"crypto"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

type DockerProvisioner interface {
	Cluster() *cluster.Cluster
	Collection() *storage.Collection
	PushImage(name, tag string) error
	ActionLimiter() provision.ActionLimiter
	GetNodeByHost(host string) (cluster.Node, error)
}

type SchedulerOpts struct {
	AppName       string
	ProcessName   string
	UpdateName    bool
	ActionLimiter provision.ActionLimiter
	LimiterDone   func()
}

type SchedulerError struct {
	Base error
}

func (e *SchedulerError) Error() string {
	return fmt.Sprintf("error in scheduler: %s", e.Base)
}

type Container struct {
	types.Container `bson:",inline"`
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
	Provisioner      DockerProvisioner
	DestinationHosts []string
	ProcessName      string
	Deploy           bool
	Building         bool
}

func (c *Container) Create(args *CreateArgs) error {
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
	var nodeList []string
	if len(args.DestinationHosts) > 0 {
		var node cluster.Node
		node, err = args.Provisioner.GetNodeByHost(args.DestinationHosts[0])
		if err != nil {
			return err
		}
		nodeList = []string{node.Address}
	}
	schedulerOpts := &SchedulerOpts{
		AppName:       args.App.GetName(),
		ProcessName:   args.ProcessName,
		UpdateName:    true,
		ActionLimiter: args.Provisioner.ActionLimiter(),
	}
	addr, cont, err := args.Provisioner.Cluster().CreateContainerSchedulerOpts(opts, schedulerOpts, net.StreamInactivityTimeout, nodeList...)
	hostAddr := net.URLToHost(addr)
	if schedulerOpts.LimiterDone != nil {
		schedulerOpts.LimiterDone()
	}
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

func (c *Container) NetworkInfo(p DockerProvisioner) (NetworkInfo, error) {
	var netInfo NetworkInfo
	dockerContainer, err := p.Cluster().InspectContainer(c.ID)
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

func (c *Container) SetStatus(p DockerProvisioner, status provision.Status, updateDB bool) error {
	c.Status = status.String()
	c.LastStatusUpdate = time.Now().In(time.UTC)
	if c.Status != provision.StatusError.String() {
		c.StatusBeforeError = c.Status
	}
	updateData := bson.M{
		"status":            c.Status,
		"statusbeforeerror": c.StatusBeforeError,
		"laststatusupdate":  c.LastStatusUpdate,
	}
	if c.Status == provision.StatusStarted.String() ||
		c.Status == provision.StatusStarting.String() ||
		c.Status == provision.StatusStopped.String() {
		c.LastSuccessStatusUpdate = c.LastStatusUpdate
		updateData["lastsuccessstatusupdate"] = c.LastSuccessStatusUpdate
	}
	if !updateDB {
		return nil
	}
	coll := p.Collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": c.ID, "status": bson.M{"$ne": provision.StatusBuilding.String()}}, bson.M{"$set": updateData})
}

func (c *Container) SetImage(p DockerProvisioner, imageID string) error {
	c.Image = imageID
	coll := p.Collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": c.ID}, c)
}

func (c *Container) Remove(p DockerProvisioner) error {
	log.Debugf("Removing container %s from docker", c.ID)
	err := c.Stop(p)
	if err != nil {
		log.Errorf("error on stop unit %s - %s", c.ID, err)
	}
	done := p.ActionLimiter().Start(c.HostAddr)
	err = p.Cluster().RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
	done()
	if err != nil {
		log.Errorf("Failed to remove container from docker: %s", err)
	}
	log.Debugf("Removing container %s from database", c.ID)
	coll := p.Collection()
	defer coll.Close()
	if err := coll.Remove(bson.M{"id": c.ID}); err != nil {
		log.Errorf("Failed to remove container from database: %s", err)
	}
	return nil
}

type Pty struct {
	Width  int
	Height int
	Term   string
}

func (c *Container) Shell(p DockerProvisioner, stdin io.Reader, stdout, stderr io.Writer, pty Pty) error {
	cmds := []string{"/usr/bin/env", "TERM=" + pty.Term, "bash", "-l"}
	execCreateOpts := docker.CreateExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmds,
		Container:    c.ID,
		Tty:          true,
	}
	exec, err := p.Cluster().CreateExec(execCreateOpts)
	if err != nil {
		return err
	}
	startExecOptions := docker.StartExecOptions{
		InputStream:  stdin,
		OutputStream: stdout,
		ErrorStream:  stderr,
		Tty:          true,
		RawTerminal:  true,
	}
	errs := make(chan error, 1)
	go func() {
		errs <- p.Cluster().StartExec(exec.ID, c.ID, startExecOptions)
	}()
	execInfo, err := p.Cluster().InspectExec(exec.ID, c.ID)
	for !execInfo.Running && err == nil {
		select {
		case startErr := <-errs:
			return startErr
		default:
			execInfo, err = p.Cluster().InspectExec(exec.ID, c.ID)
		}
	}
	if err != nil {
		return err
	}
	p.Cluster().ResizeExecTTY(exec.ID, c.ID, pty.Height, pty.Width)
	return <-errs
}

type execErr struct {
	code int
}

func (e *execErr) Error() string {
	return fmt.Sprintf("unexpected exit code: %d", e.code)
}

func (c *Container) Exec(p DockerProvisioner, stdout, stderr io.Writer, cmd string, args ...string) error {
	cmds := []string{"/bin/bash", "-lc", cmd}
	cmds = append(cmds, args...)
	execCreateOpts := docker.CreateExecOptions{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          cmds,
		Container:    c.ID,
	}
	exec, err := p.Cluster().CreateExec(execCreateOpts)
	if err != nil {
		return err
	}
	startExecOptions := docker.StartExecOptions{
		OutputStream: stdout,
		ErrorStream:  stderr,
	}
	err = p.Cluster().StartExec(exec.ID, c.ID, startExecOptions)
	if err != nil {
		return err
	}
	execData, err := p.Cluster().InspectExec(exec.ID, c.ID)
	if err != nil {
		return err
	}
	if execData.ExitCode != 0 {
		return &execErr{code: execData.ExitCode}
	}
	return nil
}

// Commits commits the container, creating an image in Docker. It then returns
// the image identifier for usage in future container creation.
func (c *Container) Commit(p DockerProvisioner, writer io.Writer) (string, error) {
	log.Debugf("committing container %s", c.ID)
	parts := strings.Split(c.BuildingImage, ":")
	if len(parts) < 2 {
		return "", log.WrapError(errors.Errorf("error parsing image name, not enough parts: %s", c.BuildingImage))
	}
	repository := strings.Join(parts[:len(parts)-1], ":")
	tag := parts[len(parts)-1]
	opts := docker.CommitContainerOptions{Container: c.ID, Repository: repository, Tag: tag}
	done := p.ActionLimiter().Start(c.HostAddr)
	image, err := p.Cluster().CommitContainer(opts)
	done()
	if err != nil {
		return "", log.WrapError(errors.Wrapf(err, "error in commit container %s", c.ID))
	}
	imgHistory, err := p.Cluster().ImageHistory(c.BuildingImage)
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
	maxTry, _ := config.GetInt("docker:registry-max-try")
	if maxTry <= 0 {
		maxTry = 3
	}
	for i := 0; i < maxTry; i++ {
		err = p.PushImage(repository, tag)
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
	return c.BuildingImage, nil
}

func (c *Container) Sleep(p DockerProvisioner) error {
	if c.Status != provision.StatusStarted.String() && c.Status != provision.StatusStarting.String() {
		return errors.Errorf("container %s is not starting or started", c.ID)
	}
	done := p.ActionLimiter().Start(c.HostAddr)
	err := p.Cluster().StopContainer(c.ID, 10)
	done()
	if err != nil {
		log.Errorf("error on stop container %s: %s", c.ID, err)
	}
	return c.SetStatus(p, provision.StatusAsleep, true)
}

func (c *Container) Stop(p DockerProvisioner) error {
	if c.Status == provision.StatusStopped.String() {
		return nil
	}
	done := p.ActionLimiter().Start(c.HostAddr)
	err := p.Cluster().StopContainer(c.ID, 10)
	done()
	if err != nil {
		log.Errorf("error on stop container %s: %s", c.ID, err)
	}
	c.SetStatus(p, provision.StatusStopped, true)
	return nil
}

type StartArgs struct {
	Provisioner DockerProvisioner
	App         provision.App
	Deploy      bool
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
	return &hostConfig, nil
}

func (c *Container) Start(args *StartArgs) error {
	done := args.Provisioner.ActionLimiter().Start(c.HostAddr)
	err := args.Provisioner.Cluster().StartContainer(c.ID, nil)
	done()
	if err != nil {
		return err
	}
	initialStatus := provision.StatusStarting
	if args.Deploy {
		initialStatus = provision.StatusBuilding
	}
	return c.SetStatus(args.Provisioner, initialStatus, false)
}

func (c *Container) Logs(p DockerProvisioner, w io.Writer) (int, error) {
	container, err := p.Cluster().InspectContainer(c.ID)
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
	return SafeAttachWaitContainer(p, opts)
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

func SafeAttachWaitContainer(p DockerProvisioner, opts docker.AttachToContainerOptions) (int, error) {
	cluster := p.Cluster()
	resultCh := make(chan waitResult, 1)
	go func() {
		err := cluster.AttachToContainer(opts)
		if err != nil {
			resultCh <- waitResult{err: err}
			return
		}
		status, err := cluster.WaitContainer(opts.Container)
		resultCh <- waitResult{status: status, err: err}
	}()
	for {
		select {
		case result := <-resultCh:
			return result.status, result.err
		case <-time.After(safeAttachInspectTimeout):
		}
		contData, err := cluster.InspectContainer(opts.Container)
		if err != nil {
			return 0, err
		}
		if !contData.State.Running {
			return contData.State.ExitCode, nil
		}
	}
}
