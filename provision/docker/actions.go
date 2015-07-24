// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"gopkg.in/mgo.v2/bson"
)

type runContainerActionsArgs struct {
	app              provision.App
	processName      string
	imageID          string
	commands         []string
	destinationHosts []string
	writer           io.Writer
	isDeploy         bool
	buildingImage    string
	provisioner      *dockerProvisioner
}

type containersToAdd struct {
	Quantity int
	Status   provision.Status
}

type changeUnitsPipelineArgs struct {
	app         provision.App
	writer      io.Writer
	toAdd       map[string]*containersToAdd
	toRemove    []container
	toHost      string
	imageId     string
	provisioner *dockerProvisioner
	appDestroy  bool
}

func runInContainers(containers []container, callback func(*container, chan *container) error, rollback func(*container), parallel bool) error {
	if len(containers) == 0 {
		return nil
	}
	workers, _ := config.GetInt("docker:max-workers")
	if workers == 0 {
		workers = len(containers)
	}
	step := len(containers)/workers + 1
	toRollback := make(chan *container, len(containers))
	errors := make(chan error, len(containers))
	var wg sync.WaitGroup
	runFunc := func(start, end int) error {
		defer wg.Done()
		for i := start; i < end; i++ {
			err := callback(&containers[i], toRollback)
			if err != nil {
				errors <- err
				return err
			}
		}
		return nil
	}
	for i := 0; i < len(containers); i += step {
		end := i + step
		if end > len(containers) {
			end = len(containers)
		}
		wg.Add(1)
		if parallel {
			go runFunc(i, end)
		} else {
			err := runFunc(i, end)
			if err != nil {
				break
			}
		}
	}
	wg.Wait()
	close(errors)
	close(toRollback)
	if err := <-errors; err != nil {
		if rollback != nil {
			for c := range toRollback {
				rollback(c)
			}
		}
		return err
	}
	return nil
}

var insertEmptyContainerInDB = action.Action{
	Name: "insert-empty-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		contName := args.app.GetName() + "-" + randomString()
		cont := container{
			AppName:       args.app.GetName(),
			ProcessName:   args.processName,
			Type:          args.app.GetPlatform(),
			Name:          contName,
			Status:        provision.StatusCreated.String(),
			Image:         args.imageID,
			BuildingImage: args.buildingImage,
		}
		coll := args.provisioner.collection()
		defer coll.Close()
		if err := coll.Insert(cont); err != nil {
			log.Errorf("error on inserting container into database %s - %s", cont.Name, err)
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		args := ctx.Params[0].(runContainerActionsArgs)
		coll := args.provisioner.collection()
		defer coll.Close()
		coll.Remove(bson.M{"name": c.Name})
	},
}

var updateContainerInDB = action.Action{
	Name: "update-database-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(runContainerActionsArgs)
		coll := args.provisioner.collection()
		defer coll.Close()
		cont := ctx.Previous.(container)
		err := coll.Update(bson.M{"name": cont.Name}, cont)
		if err != nil {
			log.Errorf("error on updating container into database %s - %s", cont.ID, err)
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
	},
}

var createContainer = action.Action{
	Name: "create-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		cont := ctx.Previous.(container)
		args := ctx.Params[0].(runContainerActionsArgs)
		log.Debugf("create container for app %s, based on image %s, with cmds %s", args.app.GetName(), args.imageID, args.commands)
		err := cont.create(args)
		if err != nil {
			log.Errorf("error on create container for app %s - %s", args.app.GetName(), err)
			return nil, err
		}
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		args := ctx.Params[0].(runContainerActionsArgs)
		err := args.provisioner.getCluster().RemoveContainer(docker.RemoveContainerOptions{ID: c.ID})
		if err != nil {
			log.Errorf("Failed to remove the container %q: %s", c.ID, err)
		}
	},
}

var stopContainer = action.Action{
	Name: "stop-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		cont := ctx.Previous.(container)
		cont.Status = provision.StatusStopped.String()
		return cont, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		c.Status = provision.StatusCreated.String()
	},
}

var setNetworkInfo = action.Action{
	Name: "set-network-info",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c := ctx.Previous.(container)
		args := ctx.Params[0].(runContainerActionsArgs)
		info, err := c.networkInfo(args.provisioner)
		if err != nil {
			return nil, err
		}
		c.IP = info.IP
		c.HostPort = info.HTTPHostPort
		return c, nil
	},
}

var startContainer = action.Action{
	Name: "start-container",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c := ctx.Previous.(container)
		log.Debugf("starting container %s", c.ID)
		args := ctx.Params[0].(runContainerActionsArgs)
		err := c.start(args.provisioner, args.app, args.isDeploy)
		if err != nil {
			log.Errorf("error on start container %s - %s", c.ID, err)
			return nil, err
		}
		return c, nil
	},
	Backward: func(ctx action.BWContext) {
		c := ctx.FWResult.(container)
		args := ctx.Params[0].(runContainerActionsArgs)
		err := args.provisioner.getCluster().StopContainer(c.ID, 10)
		if err != nil {
			log.Errorf("Failed to stop the container %q: %s", c.ID, err)
		}
	},
}

var rollbackNotice = func(ctx action.FWContext, err error) {
	args := ctx.Params[0].(changeUnitsPipelineArgs)
	if args.writer != nil {
		fmt.Fprintf(args.writer, "\n**** ROLLING BACK AFTER FAILURE ****\n ---> %s <---\n", err)
	}
}

var provisionAddUnitsToHost = action.Action{
	Name: "provision-add-units-to-host",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		containers, err := addContainersWithHost(&args)
		if err != nil {
			return nil, err
		}
		return containers, nil
	},
	Backward: func(ctx action.BWContext) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		containers := ctx.FWResult.([]container)
		w := args.writer
		if w == nil {
			w = ioutil.Discard
		}
		units := len(containers)
		fmt.Fprintf(w, "\n---- Destroying %d created %s ----\n", units, pluralize("unit", units))
		for _, cont := range containers {
			err := cont.remove(args.provisioner)
			if err != nil {
				log.Errorf("Error removing added container %s: %s", cont.ID, err.Error())
				continue
			}
			fmt.Fprintf(w, " ---> Destroyed unit %s [%s]\n", cont.shortID(), cont.ProcessName)
		}
	},
	OnError:   rollbackNotice,
	MinParams: 1,
}

var bindAndHealthcheck = action.Action{
	Name: "bind-and-healthcheck",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		webProcessName, err := getImageWebProcessName(args.imageId)
		if err != nil {
			log.Errorf("[WARNING] cannot get the name of the web process: %s", err)
		}
		newContainers := ctx.Previous.([]container)
		writer := args.writer
		if writer == nil {
			writer = ioutil.Discard
		}
		doHealthcheck := true
		for _, c := range args.toRemove {
			if c.Status == provision.StatusError.String() || c.Status == provision.StatusStopped.String() {
				doHealthcheck = false
				break
			}
		}
		fmt.Fprintf(writer, "\n---- Binding and checking %d new %s ----\n", len(newContainers), pluralize("unit", len(newContainers)))
		return newContainers, runInContainers(newContainers, func(c *container, toRollback chan *container) error {
			unit := c.asUnit(args.app)
			err := args.app.BindUnit(&unit)
			if err != nil {
				return err
			}
			toRollback <- c
			if doHealthcheck && c.ProcessName == webProcessName {
				err = runHealthcheck(c, writer)
				if err != nil {
					return err
				}
			}
			err = args.provisioner.runRestartAfterHooks(c, writer)
			if err != nil {
				return err
			}
			fmt.Fprintf(writer, " ---> Bound and checked unit %s [%s]\n", c.shortID(), c.ProcessName)
			return nil
		}, func(c *container) {
			unit := c.asUnit(args.app)
			err := args.app.UnbindUnit(&unit)
			if err != nil {
				log.Errorf("Unable to unbind unit %q: %s", c.ID, err)
			}
		}, true)
	},
	Backward: func(ctx action.BWContext) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		newContainers := ctx.FWResult.([]container)
		w := args.writer
		if w == nil {
			w = ioutil.Discard
		}
		units := len(newContainers)
		fmt.Fprintf(w, "\n---- Unbinding %d created %s ----\n", units, pluralize("unit", units))
		for _, c := range newContainers {
			unit := c.asUnit(args.app)
			err := args.app.UnbindUnit(&unit)
			if err != nil {
				log.Errorf("Removed binding for unit %q: %s", c.ID, err)
				continue
			}
			fmt.Fprintf(w, " ---> Unbinded unit %s [%s]\n", c.shortID(), c.ProcessName)
		}
	},
	OnError: rollbackNotice,
}

var addNewRoutes = action.Action{
	Name: "add-new-routes",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		webProcessName, err := getImageWebProcessName(args.imageId)
		if err != nil {
			log.Errorf("[WARNING] cannot get the name of the web process: %s", err)
		}
		newContainers := ctx.Previous.([]container)
		r, err := getRouterForApp(args.app)
		if err != nil {
			return nil, err
		}
		writer := args.writer
		if writer == nil {
			writer = ioutil.Discard
		}
		if len(newContainers) > 0 {
			fmt.Fprintf(writer, "\n---- Adding routes to new units ----\n")
		}
		// Beware that our routers are NOT thread safe. Hipache router
		// implementaion in particular does not care with thread safeness.
		// That's why we do not add routes concurrently here. If we wish to
		// change this in the future a comprehensive concurrent test suite
		// must be added to routers first.
		return newContainers, runInContainers(newContainers, func(c *container, toRollback chan *container) error {
			if c.ProcessName != webProcessName {
				return nil
			}
			err = r.AddRoute(c.AppName, c.getAddress())
			if err != nil {
				return err
			}
			c.routable = true
			toRollback <- c
			fmt.Fprintf(writer, " ---> Added route to unit %s [%s]\n", c.shortID(), c.ProcessName)
			return nil
		}, func(c *container) {
			r.RemoveRoute(c.AppName, c.getAddress())
		}, false)
	},
	Backward: func(ctx action.BWContext) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		newContainers := ctx.FWResult.([]container)
		r, err := getRouterForApp(args.app)
		if err != nil {
			log.Errorf("[add-new-routes:Backward] Error geting router: %s", err.Error())
		}
		w := args.writer
		if w == nil {
			w = ioutil.Discard
		}
		fmt.Fprintf(w, "\n---- Removing routes from created units ----\n")
		for _, cont := range newContainers {
			if !cont.routable {
				continue
			}
			err = r.RemoveRoute(cont.AppName, cont.getAddress())
			if err != nil {
				log.Errorf("[add-new-routes:Backward] Error removing route for %s: %s", cont.ID, err.Error())
				continue
			}
			fmt.Fprintf(w, " ---> Removed route from unit %s [%s]\n", cont.shortID(), cont.ProcessName)
		}
	},
	OnError: rollbackNotice,
}

var removeOldRoutes = action.Action{
	Name: "remove-old-routes",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		r, err := getRouterForApp(args.app)
		if err != nil {
			return nil, err
		}
		writer := args.writer
		if writer == nil {
			writer = ioutil.Discard
		}
		if len(args.toRemove) > 0 {
			fmt.Fprintf(writer, "\n---- Removing routes from old units ----\n")
		}
		return ctx.Previous, runInContainers(args.toRemove, func(c *container, toRollback chan *container) error {
			err = r.RemoveRoute(c.AppName, c.getAddress())
			if err == router.ErrRouteNotFound {
				return nil
			}
			if err != nil {
				if !args.appDestroy {
					return err
				}
				log.Errorf("ignored error removing route for %q during app %q destroy: %s", c.getAddress(), c.AppName, err)
			}
			c.routable = true
			toRollback <- c
			fmt.Fprintf(writer, " ---> Removed route from unit %s [%s]\n", c.shortID(), c.ProcessName)
			return nil
		}, func(c *container) {
			r.AddRoute(c.AppName, c.getAddress())
		}, false)
	},
	Backward: func(ctx action.BWContext) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		r, err := getRouterForApp(args.app)
		if err != nil {
			log.Errorf("[remove-old-routes:Backward] Error geting router: %s", err.Error())
		}
		w := args.writer
		if w == nil {
			w = ioutil.Discard
		}
		fmt.Fprintf(w, "\n---- Adding back routes to old units ----\n")
		for _, cont := range args.toRemove {
			if !cont.routable {
				continue
			}
			err = r.AddRoute(cont.AppName, cont.getAddress())
			if err != nil {
				log.Errorf("[remove-old-routes:Backward] Error adding back route for %s: %s", cont.ID, err.Error())
				continue
			}
			fmt.Fprintf(w, " ---> Added route to unit %s [%s]\n", cont.shortID(), cont.ProcessName)
		}
	},
	OnError:   rollbackNotice,
	MinParams: 1,
}

var provisionRemoveOldUnits = action.Action{
	Name: "provision-remove-old-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		writer := args.writer
		if writer == nil {
			writer = ioutil.Discard
		}
		total := len(args.toRemove)
		fmt.Fprintf(writer, "\n---- Removing %d old %s ----\n", total, pluralize("unit", total))
		runInContainers(args.toRemove, func(c *container, toRollback chan *container) error {
			err := c.remove(args.provisioner)
			if err != nil {
				log.Errorf("Ignored error trying to remove old container %q: %s", c.ID, err)
			}
			fmt.Fprintf(writer, " ---> Removed old unit %s [%s]\n", c.shortID(), c.ProcessName)
			return nil
		}, nil, true)
		return ctx.Previous, nil
	},
	Backward: func(ctx action.BWContext) {
	},
	OnError:   rollbackNotice,
	MinParams: 1,
}

var provisionUnbindOldUnits = action.Action{
	Name: "provision-unbind-old-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		writer := args.writer
		if writer == nil {
			writer = ioutil.Discard
		}
		total := len(args.toRemove)
		fmt.Fprintf(writer, "\n---- Unbinding %d old %s ----\n", total, pluralize("unit", total))
		runInContainers(args.toRemove, func(c *container, toRollback chan *container) error {
			unit := c.asUnit(args.app)
			err := args.app.UnbindUnit(&unit)
			if err != nil {
				log.Errorf("Ignored error trying to unbind old container %q: %s", c.ID, err)
			}
			fmt.Fprintf(writer, " ---> Removed bind for old unit %s [%s]\n", c.shortID(), c.ProcessName)
			return nil
		}, nil, true)
		return ctx.Previous, nil
	}, Backward: func(ctx action.BWContext) {
	},
	OnError:   rollbackNotice,
	MinParams: 1,
}

var followLogsAndCommit = action.Action{
	Name: "follow-logs-and-commit",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		c, ok := ctx.Previous.(container)
		if !ok {
			return nil, errors.New("Previous result must be a container.")
		}
		args := ctx.Params[0].(runContainerActionsArgs)
		err := c.logs(args.provisioner, args.writer)
		if err != nil {
			log.Errorf("error on get logs for container %s - %s", c.ID, err)
			return nil, err
		}
		status, err := args.provisioner.getCluster().WaitContainer(c.ID)
		if err != nil {
			log.Errorf("Process failed for container %q: %s", c.ID, err)
			return nil, err
		}
		if status != 0 {
			return nil, fmt.Errorf("Exit status %d", status)
		}
		fmt.Fprintf(args.writer, "\n---- Building application image ----\n")
		imageId, err := c.commit(args.provisioner, args.writer)
		if err != nil {
			log.Errorf("error on commit container %s - %s", c.ID, err)
			return nil, err
		}
		fmt.Fprintf(args.writer, " ---> Cleaning up\n")
		c.remove(args.provisioner)
		return imageId, nil
	},
	Backward: func(ctx action.BWContext) {
	},
	MinParams: 1,
}

var updateAppImage = action.Action{
	Name: "update-app-image",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		args := ctx.Params[0].(changeUnitsPipelineArgs)
		currentImageName, _ := appCurrentImageName(args.app.GetName())
		if currentImageName != args.imageId {
			err := appendAppImageName(args.app.GetName(), args.imageId)
			if err != nil {
				return nil, fmt.Errorf("unable to save image name: %s", err.Error())
			}
		}
		imgHistorySize := imageHistorySize()
		allImages, err := listAppImages(args.app.GetName())
		if err != nil {
			log.Errorf("Couldn't list images for cleaning: %s", err.Error())
			return ctx.Previous, nil
		}
		for i, imgName := range allImages {
			if i > len(allImages)-imgHistorySize-1 {
				err := args.provisioner.getCluster().RemoveImageIgnoreLast(imgName)
				if err != nil {
					log.Debugf("Ignored error removing old image %q: %s", imgName, err.Error())
				}
				continue
			}
			args.provisioner.cleanImage(args.app.GetName(), imgName)
		}
		return ctx.Previous, nil
	},
	OnError: rollbackNotice,
}
