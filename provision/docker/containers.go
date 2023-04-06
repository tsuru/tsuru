// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

const (
	lockWaitTimeout = 30 * time.Second
)

type appLocker struct {
	mut      sync.Mutex
	refCount map[string]int
	evtMap   map[string]*event.Event
}

func (l *appLocker) Lock(appName string) bool {
	l.mut.Lock()
	defer l.mut.Unlock()
	if l.refCount == nil {
		l.refCount = make(map[string]int)
	}
	if l.evtMap == nil {
		l.evtMap = make(map[string]*event.Event)
	}
	if l.refCount[appName] > 0 {
		l.refCount[appName]++
		return true
	}
	evt, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeApp, Value: appName},
		InternalKind: "container-move",
		Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, appName)),
		RetryTimeout: lockWaitTimeout,
	})
	if err != nil {
		return false
	}
	l.evtMap[appName] = evt
	l.refCount[appName]++
	return true
}

func (l *appLocker) Unlock(appName string) {
	l.mut.Lock()
	defer l.mut.Unlock()
	if l.refCount == nil {
		return
	}
	l.refCount[appName]--
	if l.refCount[appName] <= 0 {
		l.refCount[appName] = 0
		rebuild.RoutesRebuildOrEnqueue(appName)
		if l.evtMap == nil || l.evtMap[appName] == nil {
			return
		}
		l.evtMap[appName].Abort()
	}
}

func (p *dockerProvisioner) HandleMoveErrors(moveErrors chan error, writer io.Writer) error {
	multiErr := tsuruErrors.NewMultiError()
	for err := range moveErrors {
		multiErr.Add(err)
		err = errors.Wrap(err, "Error moving container")
		log.Error(err)
		fmt.Fprintf(writer, "%s\n", err)
	}
	if multiErr.Len() > 0 {
		return multiErr
	}
	return nil
}

func (p *dockerProvisioner) runReplaceUnitsPipeline(ctx context.Context, w io.Writer, a provision.App, toAdd map[string]*containersToAdd, toRemoveContainers []container.Container, version appTypes.AppVersion, toHosts ...string) ([]container.Container, error) {
	var toHost string
	if len(toHosts) > 0 {
		toHost = toHosts[0]
	}
	if w == nil {
		w = io.Discard
	}
	evt, _ := w.(*event.Event)
	args := changeUnitsPipelineArgs{
		app:         a,
		toAdd:       toAdd,
		toRemove:    toRemoveContainers,
		toHost:      toHost,
		writer:      w,
		version:     version,
		provisioner: p,
		event:       evt,
	}
	var pipeline *action.Pipeline
	if p.isDryMode {
		pipeline = action.NewPipeline(
			&provisionAddUnitsToHost,
			&provisionRemoveOldUnits,
		)
	} else {
		pipeline = action.NewPipeline(
			&provisionAddUnitsToHost,
			&bindAndHealthcheck,
			&addNewRoutes,
			&setRouterHealthcheck,
			&removeOldRoutes,
			&updateAppImage,
			&provisionRemoveOldUnits,
			&provisionUnbindOldUnits,
		)
	}
	err := pipeline.Execute(ctx, args)
	if err != nil {
		return nil, err
	}
	return pipeline.Result().([]container.Container), nil
}

func (p *dockerProvisioner) runCreateUnitsPipeline(ctx context.Context, w io.Writer, a provision.App, toAdd map[string]*containersToAdd, version appTypes.AppVersion) ([]container.Container, error) {
	if w == nil {
		w = io.Discard
	}
	evt, _ := w.(*event.Event)
	args := changeUnitsPipelineArgs{
		app:         a,
		toAdd:       toAdd,
		writer:      w,
		version:     version,
		provisioner: p,
		event:       evt,
	}
	pipeline := action.NewPipeline(
		&provisionAddUnitsToHost,
		&bindAndHealthcheck,
		&addNewRoutes,
		&setRouterHealthcheck,
		&updateAppImage,
	)
	err := pipeline.Execute(ctx, args)
	if err != nil {
		return nil, err
	}
	return pipeline.Result().([]container.Container), nil
}

func (p *dockerProvisioner) MoveOneContainer(ctx context.Context, c container.Container, toHost string, errCh chan error, wg *sync.WaitGroup, writer io.Writer, locker container.AppLocker) container.Container {
	if wg != nil {
		defer wg.Done()
	}
	locked := locker.Lock(c.AppName)
	if !locked {
		errCh <- errors.Errorf("couldn't move %s, unable to lock %q", c.ID, c.AppName)
		return container.Container{}
	}
	defer locker.Unlock(c.AppName)
	a, err := app.GetByName(ctx, c.AppName)
	if err != nil {
		errCh <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("error getting app %q for unit %s", c.AppName, c.ID),
		}
		return container.Container{}
	}
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(ctx, a)
	if err != nil {
		errCh <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("error getting app %q version for unit %s", c.AppName, c.ID),
		}
		return container.Container{}
	}
	var destHosts []string
	var suffix string
	if toHost != "" {
		destHosts = []string{toHost}
		suffix = " -> " + toHost
	}
	if !p.isDryMode {
		fmt.Fprintf(writer, "Moving unit %s for %q from %s%s...\n", c.ID, c.AppName, c.HostAddr, suffix)
	}
	toAdd := map[string]*containersToAdd{c.ProcessName: {Quantity: 1, Status: c.ExpectedStatus()}}
	var evtClone *event.Event
	var pipelineWriter io.Writer
	evt, _ := writer.(*event.Event)
	if evt != nil {
		evtClone = evt.Clone()
		evtClone.SetLogWriter(io.Discard)
		pipelineWriter = evtClone
	}
	addedContainers, err := p.runReplaceUnitsPipeline(ctx, pipelineWriter, a, toAdd, []container.Container{c}, version, destHosts...)
	if evt != nil {
		evt.LogsFrom(evtClone)
	}
	if err != nil {
		errCh <- &tsuruErrors.CompositeError{
			Base:    err,
			Message: fmt.Sprintf("Error moving unit %s", c.ID),
		}
		return container.Container{}
	}
	prefix := "Moved unit"
	if p.isDryMode {
		prefix = "Would move unit"
	}
	fmt.Fprintf(writer, "%s %s -> %s for %q from %s -> %s\n", prefix, c.ID, addedContainers[0].ID, c.AppName, c.HostAddr, addedContainers[0].HostAddr)
	return addedContainers[0]
}

func (p *dockerProvisioner) moveContainer(ctx context.Context, contId string, toHost string, writer io.Writer) (container.Container, error) {
	cont, err := p.GetContainer(contId)
	if err != nil {
		return container.Container{}, err
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	moveErrors := make(chan error, 1)
	locker := &appLocker{}
	createdContainer := p.MoveOneContainer(ctx, *cont, toHost, moveErrors, &wg, writer, locker)
	close(moveErrors)
	return createdContainer, p.HandleMoveErrors(moveErrors, writer)
}

func (p *dockerProvisioner) moveContainerList(ctx context.Context, containers []container.Container, toHost string, writer io.Writer) error {
	locker := &appLocker{}
	moveErrors := make(chan error, len(containers))
	wg := sync.WaitGroup{}
	wg.Add(len(containers))
	for _, c := range containers {
		go p.MoveOneContainer(ctx, c, toHost, moveErrors, &wg, writer, locker)
	}
	go func() {
		wg.Wait()
		close(moveErrors)
	}()
	return p.HandleMoveErrors(moveErrors, writer)
}

func (p *dockerProvisioner) MoveContainers(ctx context.Context, fromHost, toHost string, writer io.Writer) error {
	containers, err := p.listContainersByHost(fromHost)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		fmt.Fprintf(writer, "No units to move in %s\n", fromHost)
		return nil
	}
	fmt.Fprintf(writer, "Moving %d units...\n", len(containers))
	return p.moveContainerList(ctx, containers, toHost, writer)
}

func (p *dockerProvisioner) rebalanceContainersByFilter(ctx context.Context, writer io.Writer, appFilter []string, metadataFilter map[string]string, dryRun bool) (*dockerProvisioner, error) {
	var hostsFilter []string
	if metadataFilter != nil {
		nodes, err := p.cluster.UnfilteredNodesForMetadata(metadataFilter)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			hostsFilter = append(hostsFilter, net.URLToHost(n.Address))
		}
		if len(hostsFilter) == 0 {
			fmt.Fprintf(writer, "No hosts matching metadata filters\n")
			return nil, nil
		}
	}
	containers, err := p.listContainersByAppAndHost(appFilter, hostsFilter)
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		fmt.Fprintf(writer, "No containers found to rebalance\n")
		return nil, nil
	}
	if dryRun {
		p, err = p.dryMode(containers)
		if err != nil {
			return nil, err
		}
		defer p.stopDryMode()
	} else {
		// Create isolated provisioner, this allow us to use modify the
		// scheduler to exclude old containers.
		p, err = p.cloneProvisioner(containers)
		if err != nil {
			return nil, err
		}
	}
	fmt.Fprintf(writer, "Rebalancing %d units...\n", len(containers))
	return p, p.moveContainerList(ctx, containers, "", writer)
}

func (p *dockerProvisioner) rebalanceContainersByHost(ctx context.Context, address string, w io.Writer) error {
	containers, err := p.listContainersByHost(address)
	if err != nil {
		return err
	}
	return p.moveContainerList(ctx, containers, "", w)
}

func (p *dockerProvisioner) runCommandInContainer(ctx context.Context, version appTypes.AppVersion, app provision.App, stdin io.Reader, stdout, stderr io.Writer, pty container.Pty, cmds ...string) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	var envs []string
	for _, e := range provision.EnvsForApp(app, "", false, version) {
		envs = append(envs, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}
	labelSet, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: app,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Provisioner:   provisionerName,
			IsIsolatedRun: true,
		},
	})
	if err != nil {
		return err
	}
	createOptions := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			AttachStdin:  stdin != nil,
			OpenStdin:    stdin != nil,
			Image:        version.VersionInfo().DeployImage,
			Entrypoint:   cmds,
			Cmd:          []string{},
			Env:          envs,
			Tty:          stdin != nil,
			Labels:       labelSet.ToLabels(),
		},
		HostConfig: &docker.HostConfig{
			AutoRemove: true,
		},
	}
	pidsLimit, _ := config.GetInt("docker:pids-limit")
	if pidsLimit > 0 {
		limit := int64(pidsLimit)
		createOptions.HostConfig.PidsLimit = &limit
	}
	cluster := p.Cluster()
	schedOpts := &container.SchedulerOpts{
		AppName:       app.GetName(),
		ActionLimiter: p.ActionLimiter(),
	}
	pullOpts := docker.PullImageOptions{
		Repository:        createOptions.Config.Image,
		InactivityTimeout: net.StreamInactivityTimeout,
	}
	addr, cont, err := cluster.CreateContainerPullOptsSchedulerOpts(
		createOptions,
		pullOpts,
		dockercommon.RegistryAuthConfig(createOptions.Config.Image),
		schedOpts,
	)
	hostAddr := net.URLToHost(addr)
	if schedOpts.LimiterDone != nil {
		schedOpts.LimiterDone()
	}
	if err != nil {
		return err
	}
	defer func() {
		done := p.ActionLimiter().Start(hostAddr)
		cluster.RemoveContainer(docker.RemoveContainerOptions{ID: cont.ID, Force: true})
		done()
	}()
	attachOptions := docker.AttachToContainerOptions{
		Container:    cont.ID,
		OutputStream: stdout,
		ErrorStream:  stderr,
		InputStream:  stdin,
		Stream:       true,
		Stdout:       true,
		Stderr:       true,
		Stdin:        stdin != nil,
		RawTerminal:  stdin != nil,
		Success:      make(chan struct{}),
	}
	waiter, err := cluster.AttachToContainerNonBlocking(attachOptions)
	if err != nil {
		return err
	}
	<-attachOptions.Success
	close(attachOptions.Success)
	done := p.ActionLimiter().Start(hostAddr)
	err = cluster.StartContainer(cont.ID, nil)
	done()
	if err != nil {
		return err
	}
	if pty.Width != 0 && pty.Height != 0 {
		cluster.ResizeContainerTTY(cont.ID, pty.Height, pty.Width)
	}
	waiter.Wait()
	return nil
}

func (p *dockerProvisioner) runningContainersByNode(nodes []*cluster.Node) (map[string][]container.Container, error) {
	appNames, err := p.listAppsForNodes(nodes)
	if err != nil {
		return nil, err
	}
	if len(appNames) > 0 {
		appTargets := make([]event.Target, len(appNames))
		allowedCtx := make([]permTypes.PermissionContext, len(appNames))
		for i, appName := range appNames {
			appTargets[i] = event.Target{Type: event.TargetTypeApp, Value: appName}
			allowedCtx[i] = permission.Context(permTypes.CtxApp, appName)
		}
		evt, err := event.NewInternalMany(appTargets, &event.Opts{
			InternalKind: "rebalance check",
			Allowed:      event.Allowed(permission.PermAppReadEvents, allowedCtx...),
			RetryTimeout: lockWaitTimeout,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "unable to lock apps for container count")
		}
		defer evt.Abort()
	}
	result := map[string][]container.Container{}
	for _, n := range nodes {
		nodeConts, err := p.listRunningContainersByHost(net.URLToHost(n.Address))
		if err != nil {
			return nil, err
		}
		result[n.Address] = nodeConts
	}
	return result, nil
}

func (p *dockerProvisioner) containerGapInNodes(nodes []*cluster.Node) (int, int, error) {
	maxCount := 0
	minCount := -1
	totalCount := 0
	containersMap, err := p.runningContainersByNode(nodes)
	if err != nil {
		return 0, 0, err
	}
	for _, containers := range containersMap {
		contCount := len(containers)
		if contCount > maxCount {
			maxCount = contCount
		}
		if minCount == -1 || contCount < minCount {
			minCount = contCount
		}
		totalCount += contCount
	}
	return totalCount, maxCount - minCount, nil
}
