// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/dotcloud/docker"
	dclient "github.com/fsouza/go-dockerclient"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"io"
	"labix.org/v2/mgo/bson"
	"strings"
	"sync"
)

var (
	dCluster *cluster.Cluster
	cmutext  sync.Mutex
	fsystem  fs.Fs
)

func dockerCluster() *cluster.Cluster {
	cmutext.Lock()
	defer cmutext.Unlock()
	if dCluster == nil {
		servers, _ := config.GetList("docker:servers")
		nodes := []cluster.Node{}
		for index, server := range servers {
			node := cluster.Node{
				ID:      fmt.Sprintf("server%d", index),
				Address: server,
			}
			nodes = append(nodes, node)
		}
		dCluster, _ = cluster.New(nodes...)
	}
	return dCluster
}

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}

// runCmd executes commands and log the given stdout and stderror.
func runCmd(cmd string, args ...string) (string, error) {
	out := bytes.Buffer{}
	err := executor().Execute(cmd, args, nil, &out, &out)
	log.Printf("running the cmd: %s with the args: %s", cmd, args)
	if err != nil {
		return "", &cmdError{cmd: cmd, args: args, err: err, out: out.String()}
	}
	return out.String(), nil
}

func getPort() (string, error) {
	return config.GetString("docker:run-cmd:port")
}

type container struct {
	ID       string `bson:"_id"`
	AppName  string
	Type     string
	IP       string
	Port     string
	HostPort string
	Status   string
	Version  string
	Image    string
}

func (c *container) getAddress() string {
	hostAddr, err := config.Get("docker:host-address")
	if err != nil {
		log.Printf("Failed to obtain container address: %s", err.Error())
		return ""
	}
	return fmt.Sprintf("http://%s:%s", hostAddr, c.HostPort)
}

// newContainer creates a new container in Docker and stores it in the database.
func newContainer(app provision.App, imageId string, cmds []string) (container, error) {
	cont := container{
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
	}
	port, err := getPort()
	if err != nil {
		log.Printf("error on getting port for container %s - %s", cont.AppName, port)
		return container{}, err
	}
	user, err := config.GetString("docker:ssh:user")
	if err != nil {
		log.Printf("error on getting user for container %s - %s", cont.AppName, user)
		return container{}, err
	}
	config := docker.Config{
		Image:        imageId,
		Cmd:          cmds,
		PortSpecs:    []string{port},
		User:         user,
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
	}
	_, c, err := dockerCluster().CreateContainer(&config)
	if err != nil {
		log.Printf("error on creating container in docker %s - %s", cont.AppName, err.Error())
		return container{}, err
	}
	cont.ID = c.ID
	cont.Port = port
	return cont, nil
}

// hostPort returns the host port mapped for the container.
func (c *container) hostPort() (string, error) {
	if c.Port == "" {
		return "", errors.New("Container does not contain any mapped port")
	}
	dockerContainer, err := dockerCluster().InspectContainer(c.ID)
	if err != nil {
		return "", err
	}
	if dockerContainer.NetworkSettings != nil {
		mappedPorts := dockerContainer.NetworkSettings.PortMapping
		if port, ok := mappedPorts[c.Port]; ok {
			return port, nil
		}
	}
	return "", fmt.Errorf("Container port %s is not mapped to any host port", c.Port)
}

// ip returns the ip for the container.
func (c *container) ip() (string, error) {
	dockerContainer, err := dockerCluster().InspectContainer(c.ID)
	if err != nil {
		return "", err
	}
	if dockerContainer.NetworkSettings == nil {
		msg := "Error when getting container information. NetworkSettings is missing."
		log.Print(msg)
		return "", errors.New(msg)
	}
	instanceIP := dockerContainer.NetworkSettings.IPAddress
	if instanceIP == "" {
		msg := "error: Can't get ipaddress..."
		log.Print(msg)
		return "", errors.New(msg)
	}
	log.Printf("Instance IpAddress: %s", instanceIP)
	return instanceIP, nil
}

func (c *container) setStatus(status string) error {
	c.Status = status
	coll := collection()
	defer coll.Database.Session.Close()
	return coll.UpdateId(c.ID, c)
}

func (c *container) setImage(imageId string) error {
	c.Image = imageId
	coll := collection()
	defer coll.Database.Session.Close()
	return coll.UpdateId(c.ID, c)
}

func deploy(app provision.App, version string, w io.Writer) (string, error) {
	commands, err := deployCmds(app, version)
	if err != nil {
		return "", err
	}
	imageId := getImage(app)
	actions := []*action.Action{&createContainer, &insertContainer}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(app, imageId, commands)
	if err != nil {
		log.Printf("error on execute deploy pipeline for app %s - %s", app.GetName(), err.Error())
		return "", err
	}
	c := pipeline.Result().(container)
	log.Printf("pipeline succefully executed - %s", c.ID)
	for {
		result, err := c.stopped()
		if err != nil {
			log.Printf("error on stopped for container %s - %s", c.ID, err.Error())
			return "", err
		}
		if result {
			break
		}
	}
	err = c.logs(w)
	if err != nil {
		log.Printf("error on get logs for container %s - %s", c.ID, err.Error())
		return "", err
	}
	imageId, err = c.commit()
	if err != nil {
		log.Printf("error on commit container %s - %s", c.ID, err.Error())
		return "", err
	}
	c.remove()
	// if err != nil {
	// 	return "", err
	// }
	return imageId, nil
}

func start(app provision.App, imageId string, w io.Writer) (*container, error) {
	commands, err := runCmds()
	if err != nil {
		return nil, err
	}
	actions := []*action.Action{&createContainer, &setIp, &setHostPort, &insertContainer, &addRoute}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(app, imageId, commands)
	if err != nil {
		return nil, err
	}
	c := pipeline.Result().(container)
	err = c.setImage(imageId)
	if err != nil {
		return nil, err
	}
	err = c.setStatus("running")
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// remove removes a docker container.
func (c *container) remove() error {
	address := c.getAddress()
	log.Printf("Removing container %s from docker", c.ID)
	err := dockerCluster().RemoveContainer(c.ID)
	if err != nil {
		log.Printf("Failed to remove container from docker: %s", err)
		return err
	}
	runCmd("ssh-keygen", "-R", c.IP)
	log.Printf("Removing container %s from database", c.ID)
	coll := collection()
	defer coll.Database.Session.Close()
	if err := coll.RemoveId(c.ID); err != nil {
		log.Printf("Failed to remove container from database: %s", err.Error())
		return err
	}
	r, err := getRouter()
	if err != nil {
		log.Printf("Failed to obtain router: %s", err.Error())
		return err
	}
	if err := r.RemoveRoute(c.AppName, address); err != nil {
		log.Printf("Failed to remove route: %s", err.Error())
		return err
	}
	return nil
}

func (c *container) ssh(stdout, stderr io.Writer, cmd string, args ...string) error {
	stderr = &filter{w: stderr, content: []byte("unable to resolve host")}
	user, err := config.GetString("docker:ssh:user")
	if err != nil {
		return err
	}
	sshArgs := []string{c.IP, "-l", user, "-o", "StrictHostKeyChecking no"}
	if keyFile, err := config.GetString("docker:ssh:private-key"); err == nil {
		sshArgs = append(sshArgs, "-i", keyFile)
	}
	sshArgs = append(sshArgs, "--", cmd)
	sshArgs = append(sshArgs, args...)
	return executor().Execute("ssh", sshArgs, nil, stdout, stderr)
}

// commit commits an image in docker based in the container
func (c *container) commit() (string, error) {
	opts := dclient.CommitContainerOptions{Container: c.ID}
	image, err := dockerCluster().CommitContainer(opts)
	if err != nil {
		log.Printf("Could not commit docker image: %s", err.Error())
		return "", err
	}
	return image.ID, nil
}

// stopped returns true if the container is stopped.
func (c *container) stopped() (bool, error) {
	dockerContainer, err := dockerCluster().InspectContainer(c.ID)
	if err != nil {
		log.Printf("error on get log for container %s", c.ID)
		return false, err
	}
	running := dockerContainer.State.Running
	return !running, nil
}

// logs returns logs for the container.
func (c *container) logs(w io.Writer) error {
	opts := dclient.AttachToContainerOptions{
		Container:    c.ID,
		Logs:         true,
		OutputStream: w,
	}
	return dockerCluster().AttachToContainer(opts)
}

func getContainer(id string) (*container, error) {
	var c container
	coll := collection()
	defer coll.Database.Session.Close()
	err := coll.Find(bson.M{"_id": id}).One(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func listAppContainers(appName string) ([]container, error) {
	var containers []container
	err := collection().Find(bson.M{"appname": appName}).All(&containers)
	return containers, err
}

// getImage returns the image name or id from an app.
func getImage(app provision.App) string {
	var c container
	collection().Find(bson.M{"appname": app.GetName()}).One(&c)
	if c.Image != "" {
		return c.Image
	}
	repoNamespace, err := config.GetString("docker:repository-namespace")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", repoNamespace, app.GetPlatform())
}

// removeImage removes an image from docker registry
func removeImage(imageId string) error {
	return dockerCluster().RemoveImage(imageId)
}

type cmdError struct {
	cmd  string
	args []string
	err  error
	out  string
}

func (e *cmdError) Error() string {
	command := e.cmd + " " + strings.Join(e.args, " ")
	return fmt.Sprintf("Failed to run command %q (%s): %s.", command, e.err, e.out)
}
