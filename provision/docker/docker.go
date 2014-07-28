// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"code.google.com/p/go.crypto/ssh"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage/redis"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/safe"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"
)

var (
	dCluster *cluster.Cluster
	cmutex   sync.Mutex
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

func dockerCluster() *cluster.Cluster {
	cmutex.Lock()
	defer cmutex.Unlock()
	var clusterStorage cluster.Storage
	if dCluster == nil {
		redisServer, err := config.GetString("docker:scheduler:redis-server")
		if err != nil {
			panic("docker:scheduler:redis-server is mandatory")
		}
		prefix, _ := config.GetString("docker:scheduler:redis-prefix")
		if password, err := config.GetString("docker:scheduler:redis-password"); err == nil {
			clusterStorage = redis.AuthenticatedRedis(redisServer, password, prefix)
		} else {
			clusterStorage = redis.Redis(redisServer, prefix)
		}
		var nodes []cluster.Node
		if isSegregateScheduler() {
			dCluster, _ = cluster.New(&segregatedScheduler{}, clusterStorage)
		} else {
			nodes = getDockerServers()
			dCluster, _ = cluster.New(nil, clusterStorage, nodes...)
		}
		autoHealing, _ := config.GetBool("docker:auto-healing")
		if autoHealing {
			healer := Healer{}
			dCluster.SetHealer(&healer)
		}
		activeMonitoring, _ := config.GetBool("docker:active-monitoring")
		if activeMonitoring {
			dCluster.StartActiveMonitoring(1 * time.Minute)
		}
	}
	return dCluster
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
	return host
}

func hostToNodeAddress(host string) (string, error) {
	nodes, err := dockerCluster().Nodes()
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
	ID               string
	AppName          string
	Type             string
	IP               string
	HostAddr         string
	HostPort         string
	SSHHostPort      string
	PrivateKey       string
	Status           string
	Version          string
	Image            string
	Name             string
	User             string
	LastStatusUpdate time.Time
}

// available returns true if the Status is Started or Unreachable.
func (c *container) available() bool {
	return c.Status == provision.StatusStarted.String() || c.Status == provision.StatusUnreachable.String()
}

func (c *container) getAddress() string {
	return fmt.Sprintf("http://%s:%s", c.HostAddr, c.HostPort)
}

func containerName() string {
	h := crypto.MD5.New()
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))[:20]
}

// creates a new container in Docker.
func (c *container) create(app provision.App, imageId string, cmds []string, destinationHosts ...string) error {
	port, err := getPort()
	if err != nil {
		log.Errorf("error on getting port for container %s - %s", c.AppName, port)
		return err
	}
	user, _ := config.GetString("docker:ssh:user")
	gitUnitRepo, _ := config.GetString("git:unit-repo")
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	exposedPorts := map[docker.Port]struct{}{
		docker.Port(port + "/tcp"): {},
		docker.Port("22/tcp"):      {},
	}
	config := docker.Config{
		Image:        imageId,
		Cmd:          cmds,
		User:         user,
		ExposedPorts: exposedPorts,
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
		Memory:       uint64(app.GetMemory() * 1024 * 1024),
		MemorySwap:   uint64(app.GetSwap() * 1024 * 1024),
	}
	config.Env = append(config.Env, fmt.Sprintf("TSURU_APP_DIR=%s", gitUnitRepo))
	if sharedMount != "" && sharedBasedir != "" {
		config.Volumes = map[string]struct{}{
			sharedMount: {},
		}

		config.Env = append(config.Env, fmt.Sprintf("TSURU_SHAREDFS_MOUNTPOINT=%s", sharedMount))
	}
	opts := docker.CreateContainerOptions{Name: c.Name, Config: &config}
	var nodeList []string
	if len(destinationHosts) > 0 {
		nodeName, err := hostToNodeAddress(destinationHosts[0])
		if err != nil {
			return err
		}
		nodeList = []string{nodeName}
	}
	addr, cont, err := dockerCluster().CreateContainerSchedulerOpts(opts, app.GetName(), nodeList...)
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
	SSHHostPort  string
	IP           string
}

// networkInfo returns the IP and the host port for the container.
func (c *container) networkInfo() (containerNetworkInfo, error) {
	var netInfo containerNetworkInfo
	port, err := getPort()
	if err != nil {
		return netInfo, err
	}
	dockerContainer, err := dockerCluster().InspectContainer(c.ID)
	if err != nil {
		return netInfo, err
	}
	if dockerContainer.NetworkSettings != nil {
		netInfo.IP = dockerContainer.NetworkSettings.IPAddress
		httpPort := docker.Port(port + "/tcp")
		for _, port := range dockerContainer.NetworkSettings.Ports[httpPort] {
			if port.HostPort != "" && port.HostIp != "" {
				netInfo.HTTPHostPort = port.HostPort
				break
			}
		}
		sshPort := docker.Port("22/tcp")
		for _, port := range dockerContainer.NetworkSettings.Ports[sshPort] {
			if port.HostPort != "" && port.HostIp != "" {
				netInfo.SSHHostPort = port.HostPort
				break
			}
		}
	}
	if netInfo.HTTPHostPort == "" {
		err = fmt.Errorf("Container port %s is not mapped to any host port", port)
	}
	return netInfo, err
}

func (c *container) setStatus(status string) error {
	c.Status = status
	c.LastStatusUpdate = time.Now().In(time.UTC)
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

func gitDeploy(app provision.App, version string, w io.Writer) (string, error) {
	commands, err := gitDeployCmds(app, version)
	if err != nil {
		return "", err
	}
	return deploy(app, commands, w)
}

func archiveDeploy(app provision.App, archiveURL string, w io.Writer) (string, error) {
	commands, err := archiveDeployCmds(app, archiveURL)
	if err != nil {
		return "", err
	}
	return deploy(app, commands, w)
}

func deploy(app provision.App, commands []string, w io.Writer) (string, error) {
	imageId := getImage(app)
	actions := []*action.Action{
		&insertEmptyContainerInDB,
		&createContainer,
		&startContainer,
		&updateContainerInDB,
		&followLogsAndCommit,
	}
	pipeline := action.NewPipeline(actions...)
	args := runContainerActionsArgs{
		app:      app,
		imageID:  imageId,
		commands: commands,
		writer:   w,
	}
	err := pipeline.Execute(args)
	if err != nil {
		log.Errorf("error on execute deploy pipeline for app %s - %s", app.GetName(), err)
		return "", err
	}
	return pipeline.Result().(string), nil
}

func start(app provision.App, imageId string, w io.Writer, destinationHosts ...string) (*container, error) {
	keyPair, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}
	privateKey, publicKey, err := marshalKey(keyPair)
	if err != nil {
		return nil, err
	}
	commands, err := runWithAgentCmds(app, publicKey)
	if err != nil {
		return nil, err
	}
	actions := []*action.Action{
		&insertEmptyContainerInDB,
		&createContainer,
		&startContainer,
		&updateContainerInDB,
		&setNetworkInfo,
		&addRoute,
	}
	pipeline := action.NewPipeline(actions...)
	args := runContainerActionsArgs{
		app:              app,
		imageID:          imageId,
		commands:         commands,
		destinationHosts: destinationHosts,
		privateKey:       privateKey,
	}
	err = pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	c := pipeline.Result().(container)
	err = c.setImage(imageId)
	if err != nil {
		return nil, err
	}
	err = c.setStatus(provision.StatusStarted.String())
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
	if c.PrivateKey == "" || c.SSHHostPort == "" {
		return c.legacySSH(stdout, stderr, cmd, args...)
	}
	key, err := ssh.ParseRawPrivateKey([]byte(c.PrivateKey))
	if err != nil {
		return err
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return err
	}
	host := c.HostAddr + ":" + c.SSHHostPort
	config := ssh.ClientConfig{
		Config: ssh.Config{Rand: rand.Reader},
		Auth:   []ssh.AuthMethod{ssh.PublicKeys(signer)},
		User:   c.User,
	}
	client, err := ssh.Dial("tcp", host, &config)
	if err != nil {
		return err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdout = stdout
	session.Stderr = stderr
	return session.Run(cmd + " " + strings.Join(args, " "))
}

func (c *container) legacySSH(stdout, stderr io.Writer, cmd string, args ...string) error {
	info, err := c.networkInfo()
	if err != nil {
		return err
	}
	stdout = &filter{w: stdout, content: []byte("unable to resolve host")}
	url := fmt.Sprintf("http://%s:%d/container/%s/cmd", c.HostAddr, sshAgentPort(), info.IP)
	input := cmdInput{Cmd: cmd, Args: args}
	var buf bytes.Buffer
	err = json.NewEncoder(&buf).Encode(input)
	if err != nil {
		return err
	}
	log.Debugf("Running SSH on %s:%d: %s %s", c.HostAddr, sshAgentPort(), cmd, args)
	resp, err := http.Post(url, "application/json", &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(stdout, resp.Body)
	return err
}

func (c *container) shell(stdin io.Reader, stdout, stderr io.Writer) error {
	dir, err := ioutil.TempDir("", "docker-container-"+c.ID)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	keyPath := path.Join(dir, c.ID+".pem")
	err = ioutil.WriteFile(keyPath, []byte(c.PrivateKey), 0400)
	if err != nil {
		return err
	}
	cmd := exec.Command(
		"ssh", "-q", "-i", keyPath, "-l", c.User,
		c.HostAddr, "-p", c.SSHHostPort,
		"-o", "StrictHostKeyChecking no",
		"-o", "PasswordAuthentication no",
	)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
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
		return "", fmt.Errorf("error in commit container %s: %s", c.ID, err.Error())
	}
	log.Debugf("image %s generated from container %s", image.ID, c.ID)
	err = pushImage(repository)
	if err != nil {
		return "", fmt.Errorf("error in push image %s: %s", repository, err.Error())
	}
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
	return nil
}

func (c *container) start() error {
	port, err := getPort()
	if err != nil {
		return err
	}
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedIsolation, _ := config.GetBool("docker:sharedfs:app-isolation")
	sharedSalt, _ := config.GetString("docker:sharedfs:salt")
	config := docker.HostConfig{}
	config.PortBindings = map[docker.Port][]docker.PortBinding{
		docker.Port(port + "/tcp"): {{HostIp: "", HostPort: ""}},
		docker.Port("22/tcp"):      {{HostIp: "", HostPort: ""}},
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
	err = dockerCluster().StartContainer(c.ID, &config)
	if err != nil {
		return err
	}
	return nil
}

// logs returns logs for the container.
func (c *container) logs(w io.Writer) error {
	container, err := dockerCluster().InspectContainer(c.ID)
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
	return dockerCluster().AttachToContainer(opts)
}

// getImage returns the image name or id from an app.
// when the container image is empty is returned the platform image.
// when a deploy is multiple of 10 is returned the platform image.
func getImage(app provision.App) string {
	c, err := getOneContainerByAppName(app.GetName())
	if err != nil || c.Image == "" || usePlatformImage(app) {
		return assembleImageName(app.GetPlatform())
	}
	return c.Image
}

// removeImage removes an image from docker cluster
func removeImage(imageId string) error {
	return dockerCluster().RemoveImage(imageId)
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
	if (deploys != 0 && deploys%10 == 0) || app.GetUpdatePlatform() {
		return true
	}
	return false
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
