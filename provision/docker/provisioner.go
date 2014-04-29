// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/exec"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/router"
	_ "github.com/tsuru/tsuru/router/hipache"
	_ "github.com/tsuru/tsuru/router/testing"
	"github.com/tsuru/tsuru/safe"
	"io"
	"io/ioutil"
	"sync"
	"time"
)

func init() {
	provision.Register("docker", &dockerProvisioner{})
}

var (
	execut exec.Executor
	emutex sync.Mutex
)

func executor() exec.Executor {
	emutex.Lock()
	defer emutex.Unlock()
	if execut == nil {
		execut = exec.OsExecutor{}
	}
	return execut
}

func getRouter() (router.Router, error) {
	r, err := config.GetString("docker:router")
	if err != nil {
		return nil, err
	}
	return router.Get(r)
}

type dockerProvisioner struct{}

// Provision creates a route for the container
func (p *dockerProvisioner) Provision(app provision.App) error {
	r, err := getRouter()
	if err != nil {
		log.Fatalf("Failed to get router: %s", err)
		return err
	}
	err = app.Ready()
	if err != nil {
		return err
	}
	return r.AddBackend(app.GetName())
}

func (p *dockerProvisioner) Restart(app provision.App) error {
	containers, err := listAppContainers(app.GetName())
	if err != nil {
		log.Errorf("Got error while getting app containers: %s", err)
		return err
	}
	var buf bytes.Buffer
	for _, c := range containers {
		err = c.ssh(&buf, &buf, "/var/lib/tsuru/restart")
		if err != nil {
			log.Errorf("Failed to restart %q: %s.", app.GetName(), err)
			log.Debug("Command outputs:")
			log.Debugf("out: %s", &buf)
			log.Debugf("err: %s", &buf)
			return err
		}
		buf.Reset()
	}
	return nil
}

func (*dockerProvisioner) Start(app provision.App) error {
	containers, err := listAppContainers(app.GetName())
	if err != nil {
		return errors.New(fmt.Sprintf("Got error while getting app containers: %s", err))
	}
	for _, c := range containers {
		err := c.start()
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *dockerProvisioner) Stop(app provision.App) error {
	containers, err := listAppContainers(app.GetName())
	if err != nil {
		log.Errorf("Got error while getting app containers: %s", err)
		return nil
	}
	for _, c := range containers {
		err := c.stop()
		if err != nil {
			log.Errorf("Failed to stop %q: %s", app.GetName(), err)
			return err
		}
	}
	return nil
}

func injectEnvsAndRestart(a provision.App) {
	time.Sleep(5e9)
	err := a.SerializeEnvVars()
	if err != nil {
		log.Errorf("Failed to serialize env vars: %s.", err)
	}
	var buf bytes.Buffer
	w := app.LogWriter{App: a, Writer: &buf}
	err = a.Restart(&w)
	if err != nil {
		log.Errorf("Failed to restart app %q (%s): %s.", a.GetName(), err, buf.String())
	}
}

func startInBackground(a provision.App, c container, imageId string, w io.Writer, started chan bool) {
	_, err := start(a, imageId, w)
	if err != nil {
		log.Errorf("error on start the app %s - %s", a.GetName(), err)
		started <- false
		return
	}
	if c.ID != "" {
		if a.RemoveUnit(c.ID) != nil {
			removeContainer(&c)
		}
	}
	started <- true
}

func (dockerProvisioner) Swap(app1, app2 provision.App) error {
	r, err := getRouter()
	if err != nil {
		return err
	}
	return r.Swap(app1.GetName(), app2.GetName())
}

func (p *dockerProvisioner) Deploy(a provision.App, version string, w io.Writer) error {
	imageId, err := deploy(a, version, w)
	if err != nil {
		return err
	}
	containers, err := listAppContainers(a.GetName())
	started := make(chan bool, len(containers))
	if err == nil && len(containers) > 0 {
		for _, c := range containers {
			go startInBackground(a, c, imageId, w, started)
		}
	} else {
		go startInBackground(a, container{}, imageId, w, started)
	}
	if <-started {
		fmt.Fprint(w, "\n ---> App will be restarted, please check its logs for more details...\n\n")
	} else {
		fmt.Fprint(w, "\n ---> App failed to start, please check its logs for more details...\n\n")
	}
	return nil
}

func (p *dockerProvisioner) Destroy(app provision.App) error {
	containers, _ := listAppContainers(app.GetName())
	go func(c []container) {
		var containersGroup sync.WaitGroup
		containersGroup.Add(len(containers))
		for _, c := range containers {
			go func(c container) {
				defer containersGroup.Done()
				err := removeContainer(&c)
				if err != nil {
					log.Error(err.Error())
				}
			}(c)
		}
		containersGroup.Wait()
		err := removeImage(assembleImageName(app.GetName()))
		if err != nil {
			log.Error(err.Error())
		}
	}(containers)
	r, err := getRouter()
	if err != nil {
		log.Errorf("Failed to get router: %s", err)
		return err
	}
	return r.RemoveBackend(app.GetName())
}

func (*dockerProvisioner) Addr(app provision.App) (string, error) {
	r, err := getRouter()
	if err != nil {
		log.Errorf("Failed to get router: %s", err)
		return "", err
	}
	addr, err := r.Addr(app.GetName())
	if err != nil {
		log.Errorf("Failed to obtain app %s address: %s", app.GetName(), err)
		return "", err
	}
	return addr, nil
}

func addUnitsWithHost(a provision.App, units uint, destinationHost ...string) ([]provision.Unit, error) {
	if units == 0 {
		return nil, errors.New("Cannot add 0 units")
	}
	length, err := getContainerCountForAppName(a.GetName())
	if err != nil {
		return nil, err
	}
	if length < 1 {
		return nil, errors.New("New units can only be added after the first deployment")
	}
	writer := app.LogWriter{App: a, Writer: ioutil.Discard}
	result := make([]provision.Unit, int(units))
	container, err := getOneContainerByAppName(a.GetName())
	if err != nil {
		return nil, err
	}
	imageId := container.Image
	for i := uint(0); i < units; i++ {
		container, err := start(a, imageId, &writer, destinationHost...)
		if err != nil {
			return nil, err
		}
		result[i] = provision.Unit{
			Name:    container.ID,
			AppName: a.GetName(),
			Type:    a.GetPlatform(),
			Ip:      container.HostAddr,
			Status:  provision.StatusBuilding,
		}
	}
	return result, nil
}

func (*dockerProvisioner) AddUnits(a provision.App, units uint) ([]provision.Unit, error) {
	return addUnitsWithHost(a, units)
}

func (*dockerProvisioner) RemoveUnit(a provision.App, unitName string) error {
	container, err := getContainer(unitName)
	if err != nil {
		return err
	}
	if container.AppName != a.GetName() {
		return errors.New("Unit does not belong to this app")
	}
	if err := removeContainer(container); err != nil {
		return err
	}
	return rebindWhenNeed(a.GetName(), container)
}

// rebindWhenNeed rebinds a unit to the app's services when it finds
// that the unit being removed has the same host that any
// of the units that still being used
func rebindWhenNeed(appName string, container *container) error {
	containers, err := listAppContainers(appName)
	if err != nil {
		return err
	}
	for _, c := range containers {
		if c.HostAddr == container.HostAddr && c.ID != container.ID {
			msg := queue.Message{Action: app.BindService, Args: []string{appName, c.ID}}
			go app.Enqueue(msg)
			break
		}
	}
	return nil
}

func removeContainer(c *container) error {
	err := c.stop()
	if err != nil {
		log.Errorf("error on stop unit %s - %s", c.ID, err)
	}
	err = c.remove()
	if err != nil {
		log.Errorf("error on remove container %s - %s", c.ID, err)
	}
	return err
}

func (*dockerProvisioner) InstallDeps(app provision.App, w io.Writer) error {
	return nil
}

func (*dockerProvisioner) ExecuteCommandOnce(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	containers, err := listAppContainers(app.GetName())
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return errors.New("No containers for this app")
	}
	container := containers[0]
	return container.ssh(stdout, stderr, cmd, args...)
}

func (*dockerProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	containers, err := listAppContainers(app.GetName())
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return errors.New("No containers for this app")
	}
	for _, c := range containers {
		err = c.ssh(stdout, stderr, cmd, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *dockerProvisioner) SetCName(app provision.App, cname string) error {
	r, err := getRouter()
	if err != nil {
		return err
	}
	return r.SetCName(cname, app.GetName())
}

func (p *dockerProvisioner) UnsetCName(app provision.App, cname string) error {
	r, err := getRouter()
	if err != nil {
		return err
	}
	return r.UnsetCName(cname, app.GetName())
}

func (p *dockerProvisioner) Commands() []cmd.Command {
	return []cmd.Command{
		addNodeToSchedulerCmd{},
		removeNodeFromSchedulerCmd{},
		listNodesInTheSchedulerCmd{},
		&sshAgentCmd{},
	}
}

func (p *dockerProvisioner) AdminCommands() []cmd.Command {
	return []cmd.Command{
		&moveContainerCmd{},
		&moveContainersCmd{},
		&rebalanceContainersCmd{},
	}
}

func collection() *storage.Collection {
	name, err := config.GetString("docker:collection")
	if err != nil {
		log.Fatal(err.Error())
	}
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
}

func (p *dockerProvisioner) DeployPipeline() *action.Pipeline {
	actions := []*action.Action{
		&app.ProvisionerDeploy,
		&app.IncrementDeploy,
		&saveUnits,
		&injectEnvirons,
		&bindService,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline
}

// PlatformAdd build and push a new docker platform to register
func (p *dockerProvisioner) PlatformAdd(name string, args string) error {
	if args == "" {
		return errors.New("Dockerfile is required.")
	}
	var buf safe.Buffer
	imageName := assembleImageName(name)
	dockerCluster := dockerCluster()
	buildOptions := docker.BuildImageOptions{
		Name:         imageName,
		Remote:       args,
		InputStream:  nil,
		OutputStream: &buf,
	}
	err := dockerCluster.BuildImage(buildOptions)
	if err != nil {
		return err
	}
	return pushImage(imageName)
}
