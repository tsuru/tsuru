// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

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
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
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
	StatusBeforeError       string
	Version                 string
	Image                   string
	Name                    string
	User                    string
	BuildingImage           string
	LastStatusUpdate        time.Time
	LastSuccessStatusUpdate time.Time
	LockedUntil             time.Time
	Routable                bool `bson:"-"`
	ExposedPort             string
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

type CreateContainerArgs struct {
	ImageID          string
	Commands         []string
	App              provision.App
	Client           provision.BuilderDockerClient
	DestinationHosts []string
	ProcessName      string
	Deploy           bool
	Building         bool
}

func (c *Container) Create(args *CreateContainerArgs) error {
	securityOpts, _ := config.GetList("docker:security-opts")
	var exposedPorts map[docker.Port]struct{}
	var user string
	if args.Building {
		user, _ = dockercommon.UserForContainer()
	}
	hostConf, err := c.hostConfig(args.App, args.Deploy)
	if err != nil {
		return err
	}
	labelSet, err := provision.ProcessLabels(provision.ProcessLabelsOpts{
		App:     args.App,
		Process: c.ProcessName,
		Builder: "docker",
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
		SecurityOpts: securityOpts,
		User:         user,
		Labels:       labelSet.ToLabels(),
	}
	c.addEnvsToConfig(args, strings.TrimSuffix(c.ExposedPort, "/tcp"), &conf)
	opts := docker.CreateContainerOptions{Name: c.Name, Config: &conf, HostConfig: hostConf}
	cont, err := args.Client.CreateContainer(opts)
	if err != nil {
		log.Errorf("error on creating container in docker %s - %s", c.AppName, err)
		return err
	}
	c.ID = cont.ID
	return nil
}

type StartArgs struct {
	Client provision.BuilderDockerClient
}

func (c *Container) Start(args *StartArgs) error {
	return args.Client.StartContainer(c.ID, nil)
}

func (c *Container) addEnvsToConfig(args *CreateContainerArgs, port string, cfg *docker.Config) {
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

func (c *Container) SetStatus(status provision.Status, updateDB bool) error {
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
	return nil
}

func (c *Container) Remove(client provision.BuilderDockerClient) error {
	log.Debugf("Removing container %s from docker", c.ID)
	err := c.Stop(client)
	if err != nil {
		log.Errorf("error on stop unit %s - %s", c.ID, err)
	}
	err = client.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
	if err != nil {
		log.Errorf("Failed to remove container from docker: %s", err)
	}
	return nil
}

type Pty struct {
	Width  int
	Height int
	Term   string
}

// Commits commits the container, creating an image in Docker. It then returns
// the image identifier for usage in future container creation.
func (c *Container) Commit(client provision.BuilderDockerClient, writer io.Writer) (string, error) {
	log.Debugf("committing container %s", c.ID)
	parts := strings.Split(c.BuildingImage, ":")
	if len(parts) < 2 {
		return "", log.WrapError(errors.Errorf("error parsing image name, not enough parts: %s", c.BuildingImage))
	}
	repository := strings.Join(parts[:len(parts)-1], ":")
	tag := parts[len(parts)-1]
	opts := docker.CommitContainerOptions{Container: c.ID, Repository: repository, Tag: tag}
	image, err := client.CommitContainer(opts)
	if err != nil {
		return "", log.WrapError(errors.Wrapf(err, "error in commit container %s", c.ID))
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
	maxTry, _ := config.GetInt("docker:registry-max-try")
	if maxTry <= 0 {
		maxTry = 3
	}
	for i := 0; i < maxTry; i++ {
		err = dockercommon.PushImage(client, repository, tag, dockercommon.RegistryAuthConfig())
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

func (c *Container) Stop(client provision.BuilderDockerClient) error {
	if c.Status == provision.StatusStopped.String() {
		return nil
	}
	err := client.StopContainer(c.ID, 10)
	if err != nil {
		log.Errorf("error on stop container %s: %s", c.ID, err)
	}
	c.SetStatus(provision.StatusStopped, true)
	return nil
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

func (c *Container) hostConfig(app provision.App, isDeploy bool) (*docker.HostConfig, error) {
	sharedBasedir, _ := config.GetString("docker:sharedfs:hostdir")
	sharedMount, _ := config.GetString("docker:sharedfs:mountpoint")
	sharedIsolation, _ := config.GetBool("docker:sharedfs:app-isolation")
	sharedSalt, _ := config.GetString("docker:sharedfs:salt")
	hostConfig := docker.HostConfig{
		CPUShares: int64(app.GetCpuShare()),
	}
	hostConfig.OomScoreAdj = 1000
	hostConfig.SecurityOpt, _ = config.GetList("docker:security-opts")
	hostConfig.LogConfig = docker.LogConfig{
		Type: dockercommon.JsonFileLogDriver,
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
			hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s/%s:%s:rw", sharedBasedir, appHostDir, sharedMount))
		} else {
			hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s:%s:rw", sharedBasedir, sharedMount))
		}
	}
	return &hostConfig, nil
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
