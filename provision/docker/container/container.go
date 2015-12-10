// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"crypto"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2/bson"
)

const (
	portRangeStart    = 49153
	portRangeEnd      = 65535
	portAllocMaxTries = 15
)

type DockerProvisioner interface {
	Cluster() *cluster.Cluster
	Collection() *storage.Collection
	PushImage(name, tag string) error
}

type Container struct {
	ID                      string
	AppName                 string
	ProcessName             string
	Type                    string
	IP                      string
	HostAddr                string
	HostPort                string
	PrivateKey              string
	Status                  string
	Version                 string
	Image                   string
	Name                    string
	User                    string
	BuildingImage           string
	LastStatusUpdate        time.Time
	LastSuccessStatusUpdate time.Time
	LockedUntil             time.Time
	Routable                bool `bson:"-"`
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
	Deploy           bool
	Provisioner      DockerProvisioner
	DestinationHosts []string
	ProcessName      string
}

func (c *Container) Create(args *CreateArgs) error {
	port, err := getPort()
	if err != nil {
		log.Errorf("error on getting port for container %s - %s", c.AppName, port)
		return err
	}
	user := c.user()
	securityOpts, _ := config.GetList("docker:security-opts")
	var exposedPorts map[docker.Port]struct{}
	if !args.Deploy {
		exposedPorts = map[docker.Port]struct{}{
			docker.Port(port + "/tcp"): {},
		}
	}
	config := docker.Config{
		Image:        args.ImageID,
		Cmd:          args.Commands,
		User:         user,
		ExposedPorts: exposedPorts,
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
		Memory:       args.App.GetMemory(),
		MemorySwap:   args.App.GetMemory() + args.App.GetSwap(),
		CPUShares:    int64(args.App.GetCpuShare()),
		SecurityOpts: securityOpts,
	}
	c.addEnvsToConfig(args, port, &config)
	opts := docker.CreateContainerOptions{Name: c.Name, Config: &config}
	var nodeList []string
	if len(args.DestinationHosts) > 0 {
		var nodeName string
		nodeName, err = c.hostToNodeAddress(args.Provisioner, args.DestinationHosts[0])
		if err != nil {
			return err
		}
		nodeList = []string{nodeName}
	}
	schedulerOpts := []string{args.App.GetName(), args.ProcessName}
	addr, cont, err := args.Provisioner.Cluster().CreateContainerSchedulerOpts(opts, schedulerOpts, nodeList...)
	if err != nil {
		log.Errorf("error on creating container in docker %s - %s", c.AppName, err)
		return err
	}
	c.ID = cont.ID
	c.HostAddr = net.URLToHost(addr)
	c.User = user
	return nil
}

func (c *Container) hostToNodeAddress(p DockerProvisioner, host string) (string, error) {
	nodes, err := p.Cluster().Nodes()
	if err != nil {
		return "", err
	}
	for _, node := range nodes {
		if net.URLToHost(node.Address) == host {
			return node.Address, nil
		}
	}
	return "", fmt.Errorf("Host `%s` not found", host)
}

func (c *Container) addEnvsToConfig(args *CreateArgs, port string, cfg *docker.Config) {
	if !args.Deploy {
		for _, envData := range args.App.Envs() {
			cfg.Env = append(cfg.Env, fmt.Sprintf("%s=%s", envData.Name, envData.Value))
		}
		cfg.Env = append(cfg.Env, fmt.Sprintf("%s=%s", "TSURU_PROCESSNAME", c.ProcessName))
	}
	host, _ := config.GetString("host")
	cfg.Env = append(cfg.Env, []string{
		fmt.Sprintf("%s=%s", "port", port),
		fmt.Sprintf("%s=%s", "PORT", port),
		fmt.Sprintf("%s=%s", "TSURU_HOST", host),
	}...)
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	if sharedMount != "" && sharedBasedir != "" {
		cfg.Volumes = map[string]struct{}{
			sharedMount: {},
		}
		cfg.Env = append(cfg.Env, fmt.Sprintf("TSURU_SHAREDFS_MOUNTPOINT=%s", sharedMount))
	}
}

func (c *Container) user() string {
	user, err := config.GetString("docker:user")
	if err != nil {
		user, _ = config.GetString("docker:ssh:user")
	}
	return user
}

type NetworkInfo struct {
	HTTPHostPort string
	IP           string
}

func (c *Container) NetworkInfo(p DockerProvisioner) (NetworkInfo, error) {
	var netInfo NetworkInfo
	port, err := getPort()
	if err != nil {
		return netInfo, err
	}
	dockerContainer, err := p.Cluster().InspectContainer(c.ID)
	if err != nil {
		return netInfo, err
	}
	if dockerContainer.NetworkSettings != nil {
		netInfo.IP = dockerContainer.NetworkSettings.IPAddress
		httpPort := docker.Port(port + "/tcp")
		for _, port := range dockerContainer.NetworkSettings.Ports[httpPort] {
			if port.HostPort != "" && port.HostIP != "" {
				netInfo.HTTPHostPort = port.HostPort
				break
			}
		}
	}
	return netInfo, err
}

func (c *Container) SetStatus(p DockerProvisioner, status string, updateDB bool) error {
	c.Status = status
	c.LastStatusUpdate = time.Now().In(time.UTC)
	updateData := bson.M{
		"status":           c.Status,
		"laststatusupdate": c.LastStatusUpdate,
	}
	if c.Status == provision.StatusStarted.String() ||
		c.Status == provision.StatusStarting.String() {
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

func (c *Container) SetImage(p DockerProvisioner, imageId string) error {
	c.Image = imageId
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
	err = p.Cluster().RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
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
		User:         c.user(),
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
		User:         c.user(),
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
	log.Debugf("commiting container %s", c.ID)
	parts := strings.Split(c.BuildingImage, ":")
	if len(parts) < 2 {
		return "", log.WrapError(fmt.Errorf("error parsing image name, not enough parts: %s", c.BuildingImage))
	}
	repository := strings.Join(parts[:len(parts)-1], ":")
	tag := parts[len(parts)-1]
	opts := docker.CommitContainerOptions{Container: c.ID, Repository: repository, Tag: tag}
	image, err := p.Cluster().CommitContainer(opts)
	if err != nil {
		return "", log.WrapError(fmt.Errorf("error in commit container %s: %s", c.ID, err.Error()))
	}
	imgData, err := p.Cluster().InspectImage(c.BuildingImage)
	imgSize := ""
	if err == nil {
		imgSize = fmt.Sprintf("(%.02fMB)", float64(imgData.Size)/1024/1024)
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
			log.Errorf("error in push image %s: %s", c.BuildingImage, err.Error())
			time.Sleep(time.Second)
			continue
		}
		break
	}
	if err != nil {
		return "", log.WrapError(fmt.Errorf("error in push image %s: %s", c.BuildingImage, err.Error()))
	}
	return c.BuildingImage, nil
}

func (c *Container) Stop(p DockerProvisioner) error {
	if c.Status == provision.StatusStopped.String() {
		return nil
	}
	err := p.Cluster().StopContainer(c.ID, 10)
	if err != nil {
		log.Errorf("error on stop container %s: %s", c.ID, err)
	}
	c.SetStatus(p, provision.StatusStopped.String(), true)
	return nil
}

type StartArgs struct {
	Provisioner DockerProvisioner
	App         provision.App
	Deploy      bool
}

func (c *Container) Start(args *StartArgs) error {
	port, err := getPort()
	if err != nil {
		return err
	}
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedIsolation, _ := config.GetBool("docker:sharedfs:app-isolation")
	sharedSalt, _ := config.GetString("docker:sharedfs:salt")
	hostConfig := docker.HostConfig{
		Memory:     args.App.GetMemory(),
		MemorySwap: args.App.GetMemory() + args.App.GetSwap(),
		CPUShares:  int64(args.App.GetCpuShare()),
	}
	if !args.Deploy {
		hostConfig.RestartPolicy = docker.AlwaysRestart()
		hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
			docker.Port(port + "/tcp"): {{HostIP: "", HostPort: ""}},
		}
		hostConfig.LogConfig = docker.LogConfig{
			Type: "syslog",
			Config: map[string]string{
				"syslog-address": "udp://localhost:" + strconv.Itoa(BsSysLogPort()),
			},
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
	allocator, _ := config.GetString("docker:port-allocator")
	if allocator == "" {
		allocator = "docker"
	}
	switch allocator {
	case "tsuru":
		err = c.startWithPortSearch(args.Provisioner, &hostConfig)
	case "docker":
		err = args.Provisioner.Cluster().StartContainer(c.ID, &hostConfig)
	default:
		return fmt.Errorf("invalid docker:port-allocator: %s", allocator)
	}
	if err != nil {
		return err
	}
	initialStatus := provision.StatusStarting.String()
	if args.Deploy {
		initialStatus = provision.StatusBuilding.String()
	}
	return c.SetStatus(args.Provisioner, initialStatus, false)
}

func (c *Container) startWithPortSearch(p DockerProvisioner, hostConfig *docker.HostConfig) error {
	intenalPort, err := getPort()
	if err != nil {
		return err
	}
	retries := 0
	rand.Seed(time.Now().UTC().UnixNano())
	for port := portRangeStart; port <= portRangeEnd; {
		if retries >= portAllocMaxTries {
			break
		}
		var usedPorts map[string]struct{}
		usedPorts, err = c.usedPortsForHost(p, c.HostAddr)
		if err != nil {
			return err
		}
		var portStr string
		for ; port <= portRangeEnd; port++ {
			portStr = strconv.Itoa(port)
			if _, used := usedPorts[portStr]; !used {
				break
			}
		}
		if port > portRangeEnd {
			break
		}
		hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
			docker.Port(intenalPort + "/tcp"): {{HostIP: "", HostPort: portStr}},
		}
		randN := rand.Uint32()
		err = p.Cluster().StartContainer(c.ID, hostConfig)
		if err != nil {
			if strings.Contains(err.Error(), "already in use") ||
				strings.Contains(err.Error(), "already allocated") {
				retries++
				port += int(randN%uint32(10*retries)) + 1
				log.Debugf("[port conflict] port conflict for %s in %s with %s trying next %d - %d/%d", c.ShortID(), c.HostAddr, portStr, port, retries, portAllocMaxTries)
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("could not start container, unable to allocate port after %d retries: %s", retries, err)
}

func (c *Container) usedPortsForHost(p DockerProvisioner, hostaddr string) (map[string]struct{}, error) {
	coll := p.Collection()
	defer coll.Close()
	var usedPortsList []string
	err := coll.Find(bson.M{"hostaddr": hostaddr}).Distinct("hostport", &usedPortsList)
	if err != nil {
		return nil, err
	}
	usedPorts := map[string]struct{}{}
	for _, port := range usedPortsList {
		usedPorts[port] = struct{}{}
	}
	return usedPorts, nil
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
		AppName:     a.GetName(),
		Type:        cType,
		Ip:          c.HostAddr,
		Status:      status,
		ProcessName: c.ProcessName,
		Address:     c.Address(),
	}
}

func getPort() (string, error) {
	port, err := config.Get("docker:run-cmd:port")
	if err != nil {
		return "", err
	}
	return fmt.Sprint(port), nil
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
