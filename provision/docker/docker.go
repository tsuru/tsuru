// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"crypto"
	"encoding/json"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/docker-cluster/storage"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/safe"
	"io"
	"labix.org/v2/mgo/bson"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	dCluster *cluster.Cluster
	cmutex   sync.Mutex
	fsystem  fs.Fs
)

var (
	clusterNodes map[string]string
	segScheduler segregatedScheduler
)

func getDockerServers() []cluster.Node {
	servers, _ := config.GetList("docker:servers")
	nodes := []cluster.Node{}
	clusterNodes = make(map[string]string)
	for index, server := range servers {
		id := fmt.Sprintf("server%d", index)
		node := cluster.Node{
			ID:      id,
			Address: server,
		}
		nodes = append(nodes, node)
		clusterNodes[id] = server
	}
	return nodes
}

func dockerCluster() *cluster.Cluster {
	cmutex.Lock()
	defer cmutex.Unlock()
	var clusterStorage cluster.Storage
	if dCluster == nil {
		if redisServer, err := config.GetString("docker:scheduler:redis-server"); err == nil {
			prefix, _ := config.GetString("docker:scheduler:redis-prefix")
			if password, err := config.GetString("docker:scheduler:redis-password"); err == nil {
				clusterStorage = storage.AuthenticatedRedis(redisServer, password, prefix)
			} else {
				clusterStorage = storage.Redis(redisServer, prefix)
			}
		}
		var nodes []cluster.Node
		if segregate, _ := config.GetBool("docker:segregate"); segregate {
			dCluster, _ = cluster.New(segScheduler, clusterStorage)
		} else {
			nodes = getDockerServers()
			dCluster, _ = cluster.New(nil, clusterStorage, nodes...)
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
	log.Debugf("running the cmd: %s with the args: %s", cmd, args)
	if err != nil {
		return "", &cmdError{cmd: cmd, args: args, err: err, out: out.String()}
	}
	return out.String(), nil
}

func getPort() (string, error) {
	port, err := config.Get("docker:run-cmd:port")
	if err != nil {
		return "", err
	}
	return fmt.Sprint(port), nil
}

func getHostAddr(hostID string) string {
	var fullAddress string
	if seg, _ := config.GetBool("docker:segregate"); seg {
		node, _ := segScheduler.GetNode(hostID)
		fullAddress = node.Address
	} else {
		fullAddress = clusterNodes[hostID]
	}
	url, _ := url.Parse(fullAddress)
	host, _, _ := net.SplitHostPort(url.Host)
	return host
}

type container struct {
	ID       string
	AppName  string
	Type     string
	IP       string
	HostAddr string
	HostPort string
	Status   string
	Version  string
	Image    string
	Name     string
}

func (c *container) getAddress() string {
	return fmt.Sprintf("http://%s:%s", c.HostAddr, c.HostPort)
}

func containerName() string {
	h := crypto.MD5.New()
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))[:20]
}

// newContainer creates a new container in Docker and stores it in the database.
func newContainer(app provision.App, imageId string, cmds []string) (container, error) {
	contName := containerName()
	cont := container{
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Name:    contName,
		Status:  "created",
	}
	coll := collection()
	defer coll.Close()
	if err := coll.Insert(cont); err != nil {
		log.Errorf("error on inserting container into database %s - %s", cont.Name, err)
		return container{}, err
	}
	port, err := getPort()
	if err != nil {
		log.Errorf("error on getting port for container %s - %s", cont.AppName, port)
		return container{}, err
	}
	user, _ := config.GetString("docker:ssh:user")
	exposedPorts := make(map[docker.Port]struct{}, 1)
	p := docker.Port(fmt.Sprintf("%s/tcp", port))
	exposedPorts[p] = struct{}{}
	config := docker.Config{
		Image:        imageId,
		Cmd:          cmds,
		User:         user,
		ExposedPorts: exposedPorts,
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
	}
	opts := docker.CreateContainerOptions{Name: contName, Config: &config}
	hostID, c, err := dockerCluster().CreateContainer(opts)
	if err != nil {
		log.Errorf("error on creating container in docker %s - %s", cont.AppName, err)
		return container{}, err
	}
	cont.ID = c.ID
	cont.HostAddr = getHostAddr(hostID)
	err = coll.Update(bson.M{"name": cont.Name}, cont)
	if err != nil {
		log.Errorf("error on updating container into database %s - %s", cont.ID, err)
		return container{}, err
	}
	return cont, nil
}

func listContainersByHost(address string) ([]container, error) {
	var list []container
	coll := collection()
	defer coll.Close()
	if err := coll.Find(bson.M{"hostaddr": address}).All(&list); err != nil {
		return nil, err
	}
	return list, nil
}

func listContainersByApp(app string) ([]container, error) {
	var list []container
	coll := collection()
	defer coll.Close()
	if err := coll.Find(bson.M{"appname": app}).All(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// networkInfo returns the IP and the host port for the container.
func (c *container) networkInfo() (string, string, error) {
	port, err := getPort()
	if err != nil {
		return "", "", err
	}
	dockerContainer, err := dockerCluster().InspectContainer(c.ID)
	if err != nil {
		return "", "", err
	}
	if dockerContainer.NetworkSettings != nil {
		ip := dockerContainer.NetworkSettings.IPAddress
		p := docker.Port(fmt.Sprintf("%s/tcp", port))
		for _, port := range dockerContainer.NetworkSettings.Ports[p] {
			if port.HostPort != "" && port.HostIp != "" {
				return ip, port.HostPort, nil
			}
		}
	}
	return "", "", fmt.Errorf("Container port %s is not mapped to any host port", port)
}

func (c *container) setStatus(status string) error {
	c.Status = status
	coll := collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": c.ID}, c)
}

func (c *container) setImage(imageId string) error {
	c.Image = imageId
	coll := collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": c.ID}, c)
}

func deploy(app provision.App, version string, w io.Writer) (string, error) {
	commands, err := deployCmds(app, version)
	if err != nil {
		return "", err
	}
	imageId := getImage(app)
	actions := []*action.Action{&createContainer, &startContainer}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(app, imageId, commands)
	if err != nil {
		log.Errorf("error on execute deploy pipeline for app %s - %s", app.GetName(), err)
		return "", err
	}
	c := pipeline.Result().(container)
	err = c.logs(w)
	if err != nil {
		log.Errorf("error on get logs for container %s - %s", c.ID, err)
		return "", err
	}
	_, err = dockerCluster().WaitContainer(c.ID)
	if err != nil {
		log.Errorf("Process failed for container %q: %s", c.ID, err)
		return "", err
	}
	imageId, err = c.commit()
	if err != nil {
		log.Errorf("error on commit container %s - %s", c.ID, err)
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
	actions := []*action.Action{&createContainer, &startContainer, &setNetworkInfo, &addRoute}
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
	log.Debugf("Removing container %s from docker", c.ID)
	err := dockerCluster().RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
	if err != nil {
		log.Errorf("Failed to remove container from docker: %s", err)
	}
	c.removeHost()
	log.Debugf("Removing container %s from database", c.ID)
	coll := collection()
	defer coll.Close()
	if err := coll.Remove(bson.M{"id": c.ID}); err != nil {
		log.Errorf("Failed to remove container from database: %s", err)
	}
	r, err := getRouter()
	if err != nil {
		log.Errorf("Failed to obtain router: %s", err)
	}
	if err := r.RemoveRoute(c.AppName, address); err != nil {
		log.Errorf("Failed to remove route: %s", err)
	}
	return nil
}

func (c *container) removeHost() error {
	url := fmt.Sprintf("http://%s:%d/container/%s", c.HostAddr, sshAgentPort(), c.IP)
	request, _ := http.NewRequest("DELETE", url, nil)
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *container) ssh(stdout, stderr io.Writer, cmd string, args ...string) error {
	ip, _, err := c.networkInfo()
	if err != nil {
		return err
	}
	stdout = &filter{w: stdout, content: []byte("unable to resolve host")}
	url := fmt.Sprintf("http://%s:%d/container/%s/cmd", c.HostAddr, sshAgentPort(), ip)
	input := cmdInput{Cmd: cmd, Args: args}
	var buf bytes.Buffer
	err = json.NewEncoder(&buf).Encode(input)
	if err != nil {
		return err
	}
    log.Debugf("Running SSH on %s:%d: CMD %s  ARGS %s", c.HostAddr, sshAgentPort(), cmd, args)
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
	log.Debugf("commiting container %s", c.ID)
	repository := assembleImageName(c.AppName)
	opts := docker.CommitContainerOptions{Container: c.ID, Repository: repository}
	image, err := dockerCluster().CommitContainer(opts)
	if err != nil {
		log.Errorf("Could not commit docker image: %s", err)
		return "", err
	}
	log.Debugf("image %s generated from container %s", image.ID, c.ID)
	pushImage(repository)
	return repository, nil
}

// stop stops the container.
func (c *container) stop() error {
	if c.Status == provision.StatusStopped.String() {
		return nil
	}
	err := dockerCluster().StopContainer(c.ID, 10)
	if err != nil {
		log.Errorf("error on stop container %s: %s", c.ID, err)
	}
	c.setStatus(provision.StatusStopped.String())
	return err
}

func (c *container) start() error {
	port, err := getPort()
	if err != nil {
		return err
	}
	config := docker.HostConfig{}
	bindings := make(map[docker.Port][]docker.PortBinding)
	bindings[docker.Port(fmt.Sprintf("%s/tcp", port))] = []docker.PortBinding{
		{
			HostIp:   "",
			HostPort: "",
		},
	}
	config.PortBindings = bindings
	return dockerCluster().StartContainer(c.ID, &config)
}

// logs returns logs for the container.
func (c *container) logs(w io.Writer) error {
	opts := docker.AttachToContainerOptions{
		Container:    c.ID,
		Logs:         true,
		Stdout:       true,
		OutputStream: w,
		ErrorStream:  w,
		Stream:       true,
	}
	err := dockerCluster().AttachToContainer(opts)
	if err != nil {
		return err
	}
	opts = docker.AttachToContainerOptions{
		Container:    c.ID,
		Logs:         true,
		Stderr:       true,
		OutputStream: w,
		ErrorStream:  w,
	}
	return dockerCluster().AttachToContainer(opts)
}

// getImage returns the image name or id from an app.
// when the container image is empty is returned the platform image.
// when a deploy is multiple of 10 is returned the platform image.
func getImage(app provision.App) string {
	var c container
	coll := collection()
	defer coll.Close()
	coll.Find(bson.M{"appname": app.GetName()}).One(&c)
	if c.Image == "" {
		return assembleImageName(app.GetPlatform())
	}
	if usePlatformImage(app) {
		err := removeImage(c.Image)
		if err != nil {
			log.Error(err.Error())
		}
		return assembleImageName(app.GetPlatform())
	}
	return c.Image
}

// removeImage removes an image from docker registry
func removeImage(imageId string) error {
	removeFromRegistry(imageId)
	return dockerCluster().RemoveImage(imageId)
}

func removeFromRegistry(imageId string) {
	parts := strings.SplitN(imageId, "/", 3)
	if len(parts) > 2 {
		registryServer := parts[0]
		url := fmt.Sprintf("http://%s/v1/repositories/%s/tags", registryServer,
			strings.Join(parts[1:], "/"))
		request, err := http.NewRequest("DELETE", url, nil)
		if err == nil {
			http.DefaultClient.Do(request)
		}
	}
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

// pushImage sends the given image to the registry server defined in the
// configuration file.
func pushImage(name string) error {
	if _, err := config.GetString("docker:registry"); err == nil {
		var buf safe.Buffer
		pushOpts := docker.PushImageOptions{Name: name, OutputStream: &buf}
		err = dockerCluster().PushImage(pushOpts, docker.AuthConfiguration{})
		if err != nil {
			log.Errorf("[docker] Failed to push image %q (%s): %s", name, err, buf.String())
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

func usePlatformImage(app provision.App) bool {
	deploys := app.GetDeploys()
	if deploys != 0 && deploys%10 == 0 {
		return true
	}
	return false
}
