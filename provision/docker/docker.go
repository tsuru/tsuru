// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"crypto"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage/mongodb"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/mgo.v2/bson"
)

func getDockerServers() []cluster.Node {
	servers, _ := config.GetList("docker:servers")
	nodes := []cluster.Node{}
	for _, server := range servers {
		node := cluster.Node{
			Address: server,
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func isSegregateScheduler() bool {
	segregate, _ := config.GetBool("docker:segregate")
	return segregate
}

func buildClusterStorage() (cluster.Storage, error) {
	mongoUrl, _ := config.GetString("docker:cluster:mongo-url")
	mongoDatabase, _ := config.GetString("docker:cluster:mongo-database")
	if mongoUrl == "" || mongoDatabase == "" {
		return nil, fmt.Errorf("Cluster Storage: docker:cluster:{mongo-url,mongo-database} must be set.")
	}
	storage, err := mongodb.Mongodb(mongoUrl, mongoDatabase)
	if err != nil {
		return nil, fmt.Errorf("Cluster Storage: Unable to connect to mongodb: %s (docker:cluster:mongo-url = %q; docker:cluster:mongo-database = %q)",
			err.Error(), mongoUrl, mongoDatabase)
	}
	return storage, nil
}

func getPort() (string, error) {
	port, err := config.Get("docker:run-cmd:port")
	if err != nil {
		return "", err
	}
	return fmt.Sprint(port), nil
}

func urlToHost(urlStr string) string {
	url, _ := url.Parse(urlStr)
	host, _, _ := net.SplitHostPort(url.Host)
	if host == "" {
		return url.Host
	}
	return host
}

func (p *dockerProvisioner) hostToNodeAddress(host string) (string, error) {
	nodes, err := p.getCluster().Nodes()
	if err != nil {
		return "", err
	}
	for _, node := range nodes {
		if urlToHost(node.Address) == host {
			return node.Address, nil
		}
	}
	return "", fmt.Errorf("Host `%s` not found", host)
}

type container struct {
	ID                      string
	AppName                 string
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
	appCache                provision.App
}

func (c *container) shortID() string {
	if len(c.ID) > 10 {
		return c.ID[:10]
	}
	return c.ID
}

// available returns true if the Status is Started or Unreachable.
func (c *container) available() bool {
	return c.Status == provision.StatusStarted.String() ||
		c.Status == provision.StatusStarting.String()
}

func (c *container) getAddress() string {
	return fmt.Sprintf("http://%s:%s", c.HostAddr, c.HostPort)
}

func randomString() string {
	h := crypto.MD5.New()
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	io.CopyN(h, rand.Reader, 10)
	return fmt.Sprintf("%x", h.Sum(nil))[:20]
}

// creates a new container in Docker.
func (c *container) create(args runContainerActionsArgs) error {
	port, err := getPort()
	if err != nil {
		log.Errorf("error on getting port for container %s - %s", c.AppName, port)
		return err
	}
	user, err := config.GetString("docker:user")
	if err != nil {
		user, _ = config.GetString("docker:ssh:user")
	}
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	securityOpts, _ := config.GetList("docker:security-opts")
	var exposedPorts map[docker.Port]struct{}
	if !args.isDeploy {
		exposedPorts = map[docker.Port]struct{}{
			docker.Port(port + "/tcp"): {},
		}
	}
	config := docker.Config{
		Image:        args.imageID,
		Cmd:          args.commands,
		User:         user,
		ExposedPorts: exposedPorts,
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
		Memory:       args.app.GetMemory(),
		MemorySwap:   args.app.GetMemory() + args.app.GetSwap(),
		CPUShares:    int64(args.app.GetCpuShare()),
		SecurityOpts: securityOpts,
	}
	if sharedMount != "" && sharedBasedir != "" {
		config.Volumes = map[string]struct{}{
			sharedMount: {},
		}
		config.Env = append(config.Env, fmt.Sprintf("TSURU_SHAREDFS_MOUNTPOINT=%s", sharedMount))
	}
	opts := docker.CreateContainerOptions{Name: c.Name, Config: &config}
	var nodeList []string
	if len(args.destinationHosts) > 0 {
		nodeName, err := args.provisioner.hostToNodeAddress(args.destinationHosts[0])
		if err != nil {
			return err
		}
		nodeList = []string{nodeName}
	}
	addr, cont, err := args.provisioner.getCluster().CreateContainerSchedulerOpts(opts, args.app.GetName(), nodeList...)
	if err != nil {
		log.Errorf("error on creating container in docker %s - %s", c.AppName, err)
		return err
	}
	c.ID = cont.ID
	c.HostAddr = urlToHost(addr)
	c.User = user
	return nil
}

type containerNetworkInfo struct {
	HTTPHostPort string
	IP           string
}

// networkInfo returns the IP and the host port for the container.
func (c *container) networkInfo(p *dockerProvisioner) (containerNetworkInfo, error) {
	var netInfo containerNetworkInfo
	port, err := getPort()
	if err != nil {
		return netInfo, err
	}
	dockerContainer, err := p.getCluster().InspectContainer(c.ID)
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

func (c *container) setStatus(p *dockerProvisioner, status string, updateDB ...bool) error {
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
	if len(updateDB) > 0 && !updateDB[0] {
		return nil
	}
	coll := p.collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": c.ID}, bson.M{"$set": updateData})
}

func (c *container) setImage(p *dockerProvisioner, imageId string) error {
	c.Image = imageId
	coll := p.collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": c.ID}, c)
}

func (p *dockerProvisioner) gitDeploy(app provision.App, version string, w io.Writer) (string, error) {
	commands, err := gitDeployCmds(app, version)
	if err != nil {
		return "", err
	}
	return p.deployPipeline(app, p.getBuildImage(app), commands, w)
}

func (p *dockerProvisioner) archiveDeploy(app provision.App, image, archiveURL string, w io.Writer) (string, error) {
	commands, err := archiveDeployCmds(app, archiveURL)
	if err != nil {
		return "", err
	}
	return p.deployPipeline(app, image, commands, w)
}

func (p *dockerProvisioner) deployPipeline(app provision.App, imageId string, commands []string, w io.Writer) (string, error) {
	actions := []*action.Action{
		&insertEmptyContainerInDB,
		&createContainer,
		&startContainer,
		&updateContainerInDB,
		&followLogsAndCommit,
	}
	pipeline := action.NewPipeline(actions...)
	buildingImage, err := appNewImageName(app.GetName())
	if err != nil {
		return "", log.WrapError(fmt.Errorf("error getting new image name for app %s", app.GetName()))
	}
	args := runContainerActionsArgs{
		app:           app,
		imageID:       imageId,
		commands:      commands,
		writer:        w,
		isDeploy:      true,
		buildingImage: buildingImage,
		provisioner:   p,
	}
	err = pipeline.Execute(args)
	if err != nil {
		log.Errorf("error on execute deploy pipeline for app %s - %s", app.GetName(), err)
		return "", err
	}
	return buildingImage, nil
}

func (p *dockerProvisioner) start(app provision.App, imageId string, w io.Writer, destinationHosts ...string) (*container, error) {
	commands, err := runWithAgentCmds(app)
	if err != nil {
		return nil, err
	}
	actions := []*action.Action{
		&insertEmptyContainerInDB,
		&createContainer,
		&startContainer,
		&updateContainerInDB,
		&setNetworkInfo,
	}
	pipeline := action.NewPipeline(actions...)
	args := runContainerActionsArgs{
		app:              app,
		imageID:          imageId,
		commands:         commands,
		destinationHosts: destinationHosts,
		provisioner:      p,
	}
	err = pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	c := pipeline.Result().(container)
	err = c.setImage(p, imageId)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *container) getApp() (provision.App, error) {
	if c.appCache != nil {
		return c.appCache, nil
	}
	var err error
	c.appCache, err = app.GetByName(c.AppName)
	return c.appCache, err
}

// remove removes a docker container.
func (c *container) remove(p *dockerProvisioner) error {
	log.Debugf("Removing container %s from docker", c.ID)
	err := c.stop(p)
	if err != nil {
		log.Errorf("error on stop unit %s - %s", c.ID, err)
	}
	err = p.getCluster().RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
	if err != nil {
		log.Errorf("Failed to remove container from docker: %s", err)
	}
	log.Debugf("Removing container %s from database", c.ID)
	coll := p.collection()
	defer coll.Close()
	if err := coll.Remove(bson.M{"id": c.ID}); err != nil {
		log.Errorf("Failed to remove container from database: %s", err)
	}
	return nil
}

type pty struct {
	width  int
	height int
	term   string
}

func (c *container) shell(p *dockerProvisioner, stdin io.Reader, stdout, stderr io.Writer, pty pty) error {
	cmds := []string{"/usr/bin/env", "TERM=" + pty.term, "bash", "-l"}
	execCreateOpts := docker.CreateExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmds,
		Container:    c.ID,
		Tty:          true,
	}
	exec, err := p.getCluster().CreateExec(execCreateOpts)
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
		errs <- p.getCluster().StartExec(exec.ID, c.ID, startExecOptions)
	}()
	execInfo, err := p.getCluster().InspectExec(exec.ID, c.ID)
	for !execInfo.Running && err == nil {
		select {
		case startErr := <-errs:
			return startErr
		default:
			execInfo, err = p.getCluster().InspectExec(exec.ID, c.ID)
		}
	}
	if err != nil {
		return err
	}
	p.getCluster().ResizeExecTTY(exec.ID, c.ID, pty.height, pty.width)
	return <-errs
}

type execErr struct {
	code int
}

func (e *execErr) Error() string {
	return fmt.Sprintf("unexpected exit code: %d", e.code)
}

func (c *container) exec(p *dockerProvisioner, stdout, stderr io.Writer, cmd string, args ...string) error {
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
	exec, err := p.getCluster().CreateExec(execCreateOpts)
	if err != nil {
		return err
	}
	startExecOptions := docker.StartExecOptions{
		OutputStream: stdout,
		ErrorStream:  stderr,
	}
	err = p.getCluster().StartExec(exec.ID, c.ID, startExecOptions)
	if err != nil {
		return err
	}
	execData, err := p.getCluster().InspectExec(exec.ID, c.ID)
	if err != nil {
		return err
	}
	if execData.ExitCode != 0 {
		return &execErr{code: execData.ExitCode}
	}
	return nil

}

// commit commits an image in docker based in the container
// and returns the image repository.
func (c *container) commit(p *dockerProvisioner, writer io.Writer) (string, error) {
	log.Debugf("commiting container %s", c.ID)
	parts := strings.Split(c.BuildingImage, ":")
	if len(parts) < 2 {
		return "", log.WrapError(fmt.Errorf("error parsing image name, not enough parts: %s", c.BuildingImage))
	}
	repository := strings.Join(parts[:len(parts)-1], ":")
	tag := parts[len(parts)-1]
	opts := docker.CommitContainerOptions{Container: c.ID, Repository: repository, Tag: tag}
	image, err := p.getCluster().CommitContainer(opts)
	if err != nil {
		return "", log.WrapError(fmt.Errorf("error in commit container %s: %s", c.ID, err.Error()))
	}
	imgData, err := p.getCluster().InspectImage(c.BuildingImage)
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
		err = p.pushImage(repository, tag)
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

// stop stops the container.
func (c *container) stop(p *dockerProvisioner) error {
	if c.Status == provision.StatusStopped.String() {
		return nil
	}
	err := p.cluster.StopContainer(c.ID, 10)
	if err != nil {
		log.Errorf("error on stop container %s: %s", c.ID, err)
	}
	c.setStatus(p, provision.StatusStopped.String())
	return nil
}

func (c *container) start(p *dockerProvisioner, isDeploy bool) error {
	port, err := getPort()
	if err != nil {
		return err
	}
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedIsolation, _ := config.GetBool("docker:sharedfs:app-isolation")
	sharedSalt, _ := config.GetString("docker:sharedfs:salt")
	config := docker.HostConfig{}
	if !isDeploy {
		config.RestartPolicy = docker.AlwaysRestart()
		config.PortBindings = map[docker.Port][]docker.PortBinding{
			docker.Port(port + "/tcp"): {{HostIP: "", HostPort: ""}},
		}
	}
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
			config.Binds = append(config.Binds, fmt.Sprintf("%s/%s:%s:rw", sharedBasedir, appHostDir, sharedMount))
		} else {
			config.Binds = append(config.Binds, fmt.Sprintf("%s:%s:rw", sharedBasedir, sharedMount))
		}
	}
	err = p.getCluster().StartContainer(c.ID, &config)
	if err != nil {
		return err
	}
	initialStatus := provision.StatusStarting.String()
	if isDeploy {
		initialStatus = provision.StatusBuilding.String()
	}
	return c.setStatus(p, initialStatus, false)
}

// logs returns logs for the container.
func (c *container) logs(p *dockerProvisioner, w io.Writer) error {
	container, err := p.getCluster().InspectContainer(c.ID)
	if err != nil {
		return err
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
	return p.getCluster().AttachToContainer(opts)
}

func (c *container) asUnit(a provision.App) provision.Unit {
	return provision.Unit{
		Name:    c.ID,
		AppName: a.GetName(),
		Type:    a.GetPlatform(),
		Ip:      c.HostAddr,
		Status:  provision.StatusBuilding,
	}
}

// pushImage sends the given image to the registry server defined in the
// configuration file.
func (p *dockerProvisioner) pushImage(name, tag string) error {
	if _, err := config.GetString("docker:registry"); err == nil {
		var buf safe.Buffer
		pushOpts := docker.PushImageOptions{Name: name, Tag: tag, OutputStream: &buf}
		err = p.getCluster().PushImage(pushOpts, getRegistryAuthConfig())
		if err != nil {
			log.Errorf("[docker] Failed to push image %q (%s): %s", name, err, buf.String())
			return err
		}
	}
	return nil
}

func getRegistryAuthConfig() docker.AuthConfiguration {
	var authConfig docker.AuthConfiguration
	authConfig.Email, _ = config.GetString("docker:registry-auth:email")
	authConfig.Username, _ = config.GetString("docker:registry-auth:username")
	authConfig.Password, _ = config.GetString("docker:registry-auth:password")
	authConfig.ServerAddress, _ = config.GetString("docker:registry")
	return authConfig
}

// unitFromContainer returns a unit that represents a container.
func unitFromContainer(c container) provision.Unit {
	return provision.Unit{
		Name:    c.ID,
		AppName: c.AppName,
		Type:    c.Type,
		Status:  provision.Status(c.Status),
		Ip:      c.HostAddr,
	}
}
