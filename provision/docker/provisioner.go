// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	_ "github.com/tsuru/tsuru/router/galeb"
	_ "github.com/tsuru/tsuru/router/hipache"
	_ "github.com/tsuru/tsuru/router/routertest"
)

func init() {
	provision.Register("docker", &dockerProvisioner{})
}

func getRouterForApp(app provision.App) (router.Router, error) {
	routerName, err := app.GetRouter()
	if err != nil {
		return nil, err
	}
	return router.Get(routerName)
}

type dockerProvisioner struct{}

func (p *dockerProvisioner) StartupMessage() (string, error) {
	nodeList, err := dockerCluster().UnfilteredNodes()
	if err != nil {
		return "", err
	}
	out := "Docker provisioner reports the following nodes:\n"
	for _, node := range nodeList {
		out += fmt.Sprintf("    Docker node: %s\n", node.Address)
	}
	return out, nil
}

func (p *dockerProvisioner) Initialize() error {
	err := initDockerCluster()
	if err != nil {
		return err
	}
	return migrateImages()
}

// Provision creates a route for the container
func (p *dockerProvisioner) Provision(app provision.App) error {
	r, err := getRouterForApp(app)
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

func (*dockerProvisioner) Restart(a provision.App, w io.Writer) error {
	containers, err := listContainersByApp(a.GetName())
	if err != nil {
		return err
	}
	imageId, err := appCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	if w == nil {
		w = ioutil.Discard
	}
	writer := &app.LogWriter{App: a, Writer: w}
	_, err = runReplaceUnitsPipeline(writer, a, containers, imageId)
	return err
}

func (*dockerProvisioner) Start(app provision.App) error {
	containers, err := listContainersByApp(app.GetName())
	if err != nil {
		return errors.New(fmt.Sprintf("Got error while getting app containers: %s", err))
	}
	var wg sync.WaitGroup
	errCh := make(chan error, len(containers)+1)
	for _, c := range containers {
		wg.Add(1)
		go func(c container) {
			defer wg.Done()
			err := c.start(false)
			if err != nil {
				errCh <- err
				return
			}
			c.setStatus(provision.StatusStarting.String())
			if info, err := c.networkInfo(); err == nil {
				fixContainer(&c, info)
			}
		}(c)
	}
	wg.Wait()
	close(errCh)
	return <-errCh
}

func (p *dockerProvisioner) Stop(app provision.App) error {
	containers, err := listContainersByApp(app.GetName())
	if err != nil {
		log.Errorf("Got error while getting app containers: %s", err)
		return nil
	}
	var wg sync.WaitGroup
	errCh := make(chan error, len(containers)+1)
	for _, c := range containers {
		wg.Add(1)
		go func(c container) {
			defer wg.Done()
			err := c.stop()
			if err != nil {
				log.Errorf("Failed to stop %q: %s", app.GetName(), err)
				errCh <- err
			}
		}(c)
	}
	wg.Wait()
	close(errCh)
	return <-errCh
}

func (dockerProvisioner) Swap(app1, app2 provision.App) error {
	r, err := getRouterForApp(app1)
	if err != nil {
		return err
	}
	return r.Swap(app1.GetName(), app2.GetName())
}

func (p *dockerProvisioner) ImageDeploy(app provision.App, imageId string, w io.Writer) (string, error) {
	isValid, err := isValidAppImage(app.GetName(), imageId)
	if err != nil {
		return "", err
	}
	if !isValid {
		return "", fmt.Errorf("invalid image for app %s: %s", app.GetName(), imageId)
	}
	return imageId, p.deploy(app, imageId, w)
}

func (p *dockerProvisioner) GitDeploy(app provision.App, version string, w io.Writer) (string, error) {
	imageId, err := gitDeploy(app, version, w)
	if err != nil {
		return "", err
	}
	return imageId, p.deployAndClean(app, imageId, w)
}

func (p *dockerProvisioner) ArchiveDeploy(app provision.App, archiveURL string, w io.Writer) (string, error) {
	imageId, err := archiveDeploy(app, getBuildImage(app), archiveURL, w)
	if err != nil {
		return "", err
	}
	return imageId, p.deployAndClean(app, imageId, w)
}

func (p *dockerProvisioner) UploadDeploy(app provision.App, archiveFile io.ReadCloser, w io.Writer) (string, error) {
	defer archiveFile.Close()
	filePath := "/home/application/archive.tar.gz"
	user, _ := config.GetString("docker:ssh:user")
	options := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			AttachStdin:  true,
			OpenStdin:    true,
			StdinOnce:    true,
			User:         user,
			Image:        getBuildImage(app),
			Cmd:          []string{"/bin/bash", "-c", "cat > " + filePath},
		},
	}
	cluster := dockerCluster()
	_, container, err := dockerCluster().CreateContainerSchedulerOpts(options, app.GetName())
	if err != nil {
		return "", err
	}
	defer cluster.RemoveContainer(docker.RemoveContainerOptions{ID: container.ID, Force: true})
	err = cluster.StartContainer(container.ID, nil)
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	err = cluster.AttachToContainer(docker.AttachToContainerOptions{
		Container:    container.ID,
		OutputStream: &output,
		ErrorStream:  &output,
		InputStream:  archiveFile,
		Stream:       true,
		Stdin:        true,
		Stdout:       true,
		Stderr:       true,
	})
	if err != nil {
		return "", err
	}
	status, err := cluster.WaitContainer(container.ID)
	if err != nil {
		return "", err
	}
	if status != 0 {
		log.Errorf("Failed to deploy container from upload: %s", &output)
		return "", fmt.Errorf("container exited with status %d", status)
	}
	image, err := cluster.CommitContainer(docker.CommitContainerOptions{Container: container.ID})
	if err != nil {
		return "", err
	}
	imageId, err := archiveDeploy(app, image.ID, "file://"+filePath, w)
	if err != nil {
		return "", err
	}
	return imageId, p.deployAndClean(app, imageId, w)
}

func (p *dockerProvisioner) deployAndClean(a provision.App, imageId string, w io.Writer) error {
	err := p.deploy(a, imageId, w)
	if err != nil {
		cleanImage(a.GetName(), imageId)
	}
	return err
}

func (p *dockerProvisioner) deploy(a provision.App, imageId string, w io.Writer) error {
	containers, err := listContainersByApp(a.GetName())
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		_, err = runCreateUnitsPipeline(w, a, 1, imageId)
	} else {
		_, err = runReplaceUnitsPipeline(w, a, containers, imageId)
	}
	return err
}

func (p *dockerProvisioner) Destroy(app provision.App) error {
	containers, err := listContainersByApp(app.GetName())
	if err != nil {
		log.Errorf("Failed to list app containers: %s", err.Error())
		return err
	}
	var containersGroup sync.WaitGroup
	containersGroup.Add(len(containers))
	for _, c := range containers {
		go func(c container) {
			defer containersGroup.Done()
			unit := c.asUnit(app)
			err := app.UnbindUnit(&unit)
			if err != nil {
				log.Errorf("Unable to unbind unit %q: %s", c.ID, err)
			}
			err = removeContainer(&c)
			if err != nil {
				log.Errorf("Unable to destroy container %s: %s", c.ID, err.Error())
			}
		}(c)
	}
	containersGroup.Wait()
	images, err := listAppImages(app.GetName())
	if err != nil {
		log.Errorf("Failed to get image ids for app %s: %s", app.GetName(), err.Error())
	}
	cluster := dockerCluster()
	for _, imageId := range images {
		err := cluster.RemoveImage(imageId)
		if err != nil {
			log.Errorf("Failed to remove image %s: %s", imageId, err.Error())
		}
		err = cluster.RemoveFromRegistry(imageId)
		if err != nil {
			log.Errorf("Failed to remove image %s from registry: %s", imageId, err.Error())
		}
	}
	err = deleteAllAppImageNames(app.GetName())
	if err != nil {
		log.Errorf("Failed to remove image names from storage for app %s: %s", app.GetName(), err.Error())
	}
	r, err := getRouterForApp(app)
	if err != nil {
		log.Errorf("Failed to get router: %s", err.Error())
		return err
	}
	err = r.RemoveBackend(app.GetName())
	if err != nil {
		log.Errorf("Failed to remove route backend: %s", err.Error())
		return err
	}
	return nil
}

func (*dockerProvisioner) Addr(app provision.App) (string, error) {
	r, err := getRouterForApp(app)
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

func runRestartAfterHooks(cont *container, w io.Writer) error {
	yamlData, err := getImageTsuruYamlDataWithFallback(cont.Image, cont.AppName)
	if err != nil {
		return err
	}
	cmds := yamlData.Hooks.Restart.After
	for _, cmd := range cmds {
		err := cont.exec(w, w, cmd)
		if err != nil {
			return fmt.Errorf("couldn't execute restart:after hook %q(%s): %s", cmd, cont.shortID(), err.Error())
		}
	}
	return nil
}

func addContainersWithHost(args *changeUnitsPipelineArgs) ([]container, error) {
	a := args.app
	w := args.writer
	units := args.unitsToAdd
	imageId := args.imageId
	var destinationHost []string
	if args.toHost != "" {
		destinationHost = []string{args.toHost}
	}
	if w == nil {
		w = ioutil.Discard
	}
	wg := sync.WaitGroup{}
	createdContainers := make(chan *container, units)
	errors := make(chan error, units)
	var plural string
	if units > 1 {
		plural = "s"
	}
	fmt.Fprintf(w, "\n---- Starting %d new unit%s ----\n", units, plural)
	for i := 0; i < units; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := start(a, imageId, w, destinationHost...)
			if err != nil {
				errors <- err
				return
			}
			unit := c.asUnit(a)
			err = a.BindUnit(&unit)
			if err != nil {
				errors <- err
				return
			}
			createdContainers <- c
			err = runHealthcheck(c, w)
			if err != nil {
				errors <- err
				return
			}
			err = runRestartAfterHooks(c, w)
			if err != nil {
				errors <- err
				return
			}
			fmt.Fprintf(w, " ---> Started unit %s...\n", c.shortID())
		}()
	}
	wg.Wait()
	close(errors)
	close(createdContainers)
	if err := <-errors; err != nil {
		for c := range createdContainers {
			log.Errorf("Removing container %q due failed add units: %s", c.ID, err)
			unit := c.asUnit(a)
			errUnbind := a.UnbindUnit(&unit)
			if errUnbind != nil {
				log.Errorf("Unable to unbind unit %q: %s", c.ID, err)
			}
			errRem := removeContainer(c)
			if errRem != nil {
				log.Errorf("Unable to destroy container %q: %s", c.ID, err)
			}
		}
		return nil, err
	}
	result := make([]container, units)
	i := 0
	for c := range createdContainers {
		result[i] = *c
		i++
	}
	return result, nil
}

func (*dockerProvisioner) AddUnits(a provision.App, units uint, w io.Writer) ([]provision.Unit, error) {
	length, err := getContainerCountForAppName(a.GetName())
	if err != nil {
		return nil, err
	}
	if length < 1 {
		return nil, errors.New("New units can only be added after the first deployment")
	}
	if units == 0 {
		return nil, errors.New("Cannot add 0 units")
	}
	if w == nil {
		w = ioutil.Discard
	}
	writer := &app.LogWriter{App: a, Writer: w}
	imageId, err := appCurrentImageName(a.GetName())
	if err != nil {
		return nil, err
	}
	conts, err := runCreateUnitsPipeline(writer, a, int(units), imageId)
	if err != nil {
		return nil, err
	}
	result := make([]provision.Unit, len(conts))
	for i, c := range conts {
		result[i] = c.asUnit(a)
	}
	return result, nil
}

func (*dockerProvisioner) RemoveUnits(a provision.App, units uint) error {
	if a == nil {
		return errors.New("remove units: app should not be nil")
	}
	if units < 1 {
		return errors.New("remove units: units must be at least 1")
	}
	containers, err := listContainersByAppOrderedByStatus(a.GetName())
	if err != nil {
		return err
	}
	if units >= uint(len(containers)) {
		return errors.New("remove units: cannot remove all units from app")
	}
	var wg sync.WaitGroup
	for i := 0; i < int(units); i++ {
		wg.Add(1)
		go func(c container) {
			unit := c.asUnit(a)
			err := a.UnbindUnit(&unit)
			if err != nil {
				log.Errorf("Failed to unbind unit %q: %s", c.ID, err)
			}
			err = removeContainer(&c)
			if err != nil {
				log.Errorf("Failed to remove container %q: %s", c.ID, err)
			}
			wg.Done()
		}(containers[i])
	}
	wg.Wait()
	return nil
}

func (*dockerProvisioner) RemoveUnit(unit provision.Unit) error {
	container, err := getContainer(unit.Name)
	if err != nil {
		return err
	}
	a, err := container.getApp()
	if err != nil {
		return err
	}
	err = a.UnbindUnit(&unit)
	if err != nil {
		log.Errorf("Failed to unbind unit %q: %s", container.ID, err)
	}
	return removeContainer(container)
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

func (p *dockerProvisioner) SetUnitStatus(unit provision.Unit, status provision.Status) error {
	container, err := getContainer(unit.Name)
	if err != nil {
		return err
	}
	if container.AppName != unit.AppName {
		return errors.New("wrong app name")
	}
	return container.setStatus(status.String())
}

func (*dockerProvisioner) ExecuteCommandOnce(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	containers, err := listRunnableContainersByApp(app.GetName())
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return provision.ErrEmptyApp
	}
	container := containers[0]
	return container.exec(stdout, stderr, cmd, args...)
}

func (*dockerProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	containers, err := listRunnableContainersByApp(app.GetName())
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return provision.ErrEmptyApp
	}
	for _, c := range containers {
		err = c.exec(stdout, stderr, cmd, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *dockerProvisioner) SetCName(app provision.App, cname string) error {
	r, err := getRouterForApp(app)
	if err != nil {
		return err
	}
	return r.SetCName(cname, app.GetName())
}

func (p *dockerProvisioner) UnsetCName(app provision.App, cname string) error {
	r, err := getRouterForApp(app)
	if err != nil {
		return err
	}
	return r.UnsetCName(cname, app.GetName())
}

func (p *dockerProvisioner) AdminCommands() []cmd.Command {
	return []cmd.Command{
		&moveContainerCmd{},
		&moveContainersCmd{},
		&rebalanceContainersCmd{},
		&addNodeToSchedulerCmd{},
		&removeNodeFromSchedulerCmd{},
		&listNodesInTheSchedulerCmd{},
		addPoolToSchedulerCmd{},
		&removePoolFromSchedulerCmd{},
		listPoolsInTheSchedulerCmd{},
		addTeamsToPoolCmd{},
		removeTeamsFromPoolCmd{},
		fixContainersCmd{},
		&listHealingHistoryCmd{},
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

// PlatformAdd build and push a new docker platform to register
func (p *dockerProvisioner) PlatformAdd(name string, args map[string]string, w io.Writer) error {
	if args["dockerfile"] == "" {
		return errors.New("Dockerfile is required.")
	}
	if _, err := url.ParseRequestURI(args["dockerfile"]); err != nil {
		return errors.New("dockerfile parameter should be an url.")
	}
	imageName := platformImageName(name)
	dockerCluster := dockerCluster()
	buildOptions := docker.BuildImageOptions{
		Name:           imageName,
		NoCache:        true,
		RmTmpContainer: true,
		Remote:         args["dockerfile"],
		InputStream:    nil,
		OutputStream:   w,
	}
	err := dockerCluster.BuildImage(buildOptions)
	if err != nil {
		return err
	}
	return pushImage(imageName, "")
}

func (p *dockerProvisioner) PlatformUpdate(name string, args map[string]string, w io.Writer) error {
	return p.PlatformAdd(name, args, w)
}

func (p *dockerProvisioner) PlatformRemove(name string) error {
	err := dockerCluster().RemoveImage(platformImageName(name))
	if err != nil && err == docker.ErrNoSuchImage {
		log.Errorf("error on remove image %s from docker.", name)
		return nil
	}
	return err
}

func (p *dockerProvisioner) Units(app provision.App) []provision.Unit {
	containers, err := listContainersByApp(app.GetName())
	if err != nil {
		return nil
	}
	units := []provision.Unit{}
	for _, container := range containers {
		unit := unitFromContainer(container)
		units = append(units, unit)
	}
	return units
}

func (p *dockerProvisioner) RegisterUnit(unit provision.Unit, customData map[string]interface{}) error {
	container, err := getContainer(unit.Name)
	if err != nil {
		return err
	}
	if container.Status == provision.StatusBuilding.String() {
		if container.BuildingImage != "" && customData != nil {
			return saveImageCustomData(container.BuildingImage, customData)
		}
		return nil
	}
	err = container.setStatus(provision.StatusStarted.String())
	if err != nil {
		return err
	}
	return checkContainer(*container, nil)
}

func (p *dockerProvisioner) Shell(app provision.App, conn net.Conn, width, height int, args ...string) error {
	var (
		c   *container
		err error
	)
	if len(args) > 0 && args[0] != "" {
		c, err = getContainer(args[0])
	} else {
		c, err = getOneContainerByAppName(app.GetName())
	}
	if err != nil {
		return err
	}
	return c.shell(conn, conn, conn, pty{width: width, height: height})
}

func (p *dockerProvisioner) ValidAppImages(appName string) ([]string, error) {
	return listValidAppImages(appName)
}
