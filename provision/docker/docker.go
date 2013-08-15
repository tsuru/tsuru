// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dotcloud/docker"
	dclient "github.com/fsouza/go-dockerclient"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/docker-cluster/storage"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"io"
	"labix.org/v2/mgo/bson"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

var (
	dCluster *cluster.Cluster
	cmutext  sync.Mutex
	fsystem  fs.Fs
)

const maxTry = 5

var clusterNodes map[string]string

func dockerCluster() *cluster.Cluster {
	cmutext.Lock()
	defer cmutext.Unlock()
	if dCluster == nil {
		clusterNodes = make(map[string]string)
		servers, _ := config.GetList("docker:servers")
		if len(servers) < 1 {
			log.Fatal(`Tsuru is misconfigured. Setting "docker:servers" is mandatory`)
		}
		nodes := []cluster.Node{}
		for index, server := range servers {
			id := fmt.Sprintf("server%d", index)
			node := cluster.Node{
				ID:      id,
				Address: server,
			}
			nodes = append(nodes, node)
			clusterNodes[id] = server
		}
		if segregate, _ := config.GetBool("docker:segregate"); segregate {
			var scheduler segregatedScheduler
			dCluster, _ = cluster.New(&scheduler, nodes...)
		} else {
			dCluster, _ = cluster.New(nil, nodes...)
		}
		if redisServer, err := config.GetString("docker:scheduler:redis-server"); err == nil {
			prefix, _ := config.GetString("docker:scheduler:redis-prefix")
			if password, err := config.GetString("docker:scheduler:redis-password"); err == nil {
				dCluster.SetStorage(storage.AuthenticatedRedis(redisServer, password, prefix))
			} else {
				dCluster.SetStorage(storage.Redis(redisServer, prefix))
			}
		}
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

func getHostAddr(hostID string) string {
	fullAddress := clusterNodes[hostID]
	url, _ := url.Parse(fullAddress)
	host, _, _ := net.SplitHostPort(url.Host)
	return host
}

type container struct {
	ID       string `bson:"_id"`
	AppName  string
	Type     string
	IP       string
	Port     string
	HostAddr string
	HostPort string
	Status   string
	Version  string
	Image    string
}

func (c *container) getAddress() string {
	return fmt.Sprintf("http://%s:%s", c.HostAddr, c.HostPort)
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
	config := docker.Config{
		Image:        imageId,
		Cmd:          cmds,
		PortSpecs:    []string{port},
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
	}
	hostID, c, err := dockerCluster().CreateContainer(&config)
	if err != nil {
		log.Printf("error on creating container in docker %s - %s", cont.AppName, err.Error())
		return container{}, err
	}
	cont.ID = c.ID
	cont.Port = port
	cont.HostAddr = getHostAddr(hostID)
	return cont, nil
}

// networkInfo returns the IP and the host port for the container.
func (c *container) networkInfo() (string, string, error) {
	if c.Port == "" {
		return "", "", errors.New("Container does not contain any mapped port")
	}
	dockerContainer, err := dockerCluster().InspectContainer(c.ID)
	if err != nil {
		return "", "", err
	}
	if dockerContainer.NetworkSettings != nil {
		ip := dockerContainer.NetworkSettings.IPAddress
		mappedPorts := dockerContainer.NetworkSettings.PortMapping
		if port, ok := mappedPorts["Tcp"][c.Port]; ok {
			return ip, port, nil
		}
	}
	return "", "", fmt.Errorf("Container port %s is not mapped to any host port", c.Port)
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
		if port, ok := mappedPorts["Tcp"][c.Port]; ok {
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
	actions := []*action.Action{&createContainer, &startContainer, &insertContainer}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(app, imageId, commands)
	if err != nil {
		log.Printf("error on execute deploy pipeline for app %s - %s", app.GetName(), err.Error())
		return "", err
	}
	c := pipeline.Result().(container)
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
	return imageId, nil
}

func start(app provision.App, imageId string, w io.Writer) (*container, error) {
	commands, err := runCmds()
	if err != nil {
		return nil, err
	}
	actions := []*action.Action{&createContainer, &startContainer, &setIp, &setHostPort, &insertContainer, &addRoute}
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
	}
	runCmd("ssh-keygen", "-R", c.IP)
	log.Printf("Removing container %s from database", c.ID)
	coll := collection()
	defer coll.Database.Session.Close()
	if err := coll.RemoveId(c.ID); err != nil {
		log.Printf("Failed to remove container from database: %s", err)
	}
	r, err := getRouter()
	if err != nil {
		log.Printf("Failed to obtain router: %s", err)
	}
	if err := r.RemoveRoute(c.AppName, address); err != nil {
		log.Printf("Failed to remove route: %s", err)
	}
	return nil
}

func (c *container) ssh(stdout, stderr io.Writer, cmd string, args ...string) error {
	port, _ := config.GetInt("docker:ssh-agent-port")
	if port == 0 {
		port = 4545
	}
	stdout = &filter{w: stdout, content: []byte("unable to resolve host")}
	url := fmt.Sprintf("http://%s:%d/container/%s/cmd", c.HostAddr, port, c.IP)
	input := cmdInput{Cmd: cmd, Args: args}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(input)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(stdout, resp.Body)
	return err
}

// commit commits an image in docker based in the container
// and returns the image repository.
func (c *container) commit() (string, error) {
	log.Printf("commiting container %s", c.ID)
	repository := assembleImageName(c.AppName)
	opts := dclient.CommitContainerOptions{Container: c.ID, Repository: repository}
	image, err := dockerCluster().CommitContainer(opts)
	if err != nil {
		log.Printf("Could not commit docker image: %s", err.Error())
		return "", err
	}
	log.Printf("image %s generated from container %s", image.ID, c.ID)
	replicateImage(repository)
	return repository, nil
}

// stopped returns true if the container is stopped.
func (c *container) stopped() (bool, error) {
	dockerContainer, err := dockerCluster().InspectContainer(c.ID)
	if err != nil {
		log.Printf("error on get log for container %s: %s", c.ID, err)
		return false, err
	}
	return !dockerContainer.State.Running, nil
}

// stop stops the container.
func (c *container) stop() error {
	err := dockerCluster().StopContainer(c.ID, 10)
	if err != nil {
		log.Printf("error on stop container %s: %s", c.ID, err)
	}
	return err
}

// logs returns logs for the container.
func (c *container) logs(w io.Writer) error {
	opts := dclient.AttachToContainerOptions{
		Container:    c.ID,
		Logs:         true,
		Stdout:       true,
		OutputStream: w,
	}
	err := dockerCluster().AttachToContainer(opts)
	if err != nil {
		return err
	}
	opts = dclient.AttachToContainerOptions{
		Container:    c.ID,
		Logs:         true,
		Stderr:       true,
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
	return assembleImageName(app.GetPlatform())
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

// replicateImage replicates the given image through all nodes in the cluster.
func replicateImage(name string) error {
	var buf bytes.Buffer
	if registry, err := config.GetString("docker:registry"); err == nil {
		if !strings.HasPrefix(name, registry) {
			name = registry + "/" + name
		}
		pushOpts := dclient.PushImageOptions{Name: name}
		for i := 0; i < maxTry; i++ {
			err := dockerCluster().PushImage(pushOpts, &buf)
			if err == nil {
				buf.Reset()
				break
			}
			log.Printf("[docker] Failed to push image %q (%s): %s", name, err, buf.String())
			buf.Reset()
		}
		if err != nil {
			return err
		}
		pullOpts := dclient.PullImageOptions{Repository: name}
		for i := 0; i < maxTry; i++ {
			err = dockerCluster().PullImage(pullOpts, &buf)
			if err == nil {
				break
			}
			buf.Reset()
		}
		if err != nil {
			log.Printf("[docker] Failed to replicate image %q through nodes (%s): %s", name, err, buf.String())
			return err
		}
	}
	return nil
}

func assembleImageName(appName string) string {
	parts := make([]string, 0, 3)
	registry, _ := config.GetString("docker:registry")
	if registry != "" {
		parts = append(parts, registry)
	}
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	parts = append(parts, repoNamespace, appName)
	return strings.Join(parts, "/")
}
