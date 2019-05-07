// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	clusterLog "github.com/tsuru/docker-cluster/log"
	clusterStorage "github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/app/image/gc"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruHealer "github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/healer"
	internalNodeContainer "github.com/tsuru/tsuru/provision/docker/nodecontainer"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/node"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/queue"
	_ "github.com/tsuru/tsuru/router/api"
	_ "github.com/tsuru/tsuru/router/galeb"
	_ "github.com/tsuru/tsuru/router/galebv2"
	_ "github.com/tsuru/tsuru/router/hipache"
	_ "github.com/tsuru/tsuru/router/routertest"
	_ "github.com/tsuru/tsuru/router/vulcand"
	"github.com/tsuru/tsuru/safe"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

var (
	mainDockerProvisioner *dockerProvisioner

	ErrUnitRecreationCanceled = errors.New("unit creation canceled by user action")
)

const (
	provisionerName           = "docker"
	provisionerCollectionName = "dockercluster"
)

func init() {
	mainDockerProvisioner = &dockerProvisioner{}
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return mainDockerProvisioner, nil
	})
}

type dockerProvisioner struct {
	cluster        *cluster.Cluster
	collectionName string
	storage        cluster.Storage
	scheduler      *segregatedScheduler
	isDryMode      bool
	actionLimiter  provision.ActionLimiter
}

var (
	_ provision.Provisioner               = &dockerProvisioner{}
	_ provision.RollbackableDeployer      = &dockerProvisioner{}
	_ provision.ExecutableProvisioner     = &dockerProvisioner{}
	_ provision.SleepableProvisioner      = &dockerProvisioner{}
	_ provision.MessageProvisioner        = &dockerProvisioner{}
	_ provision.InitializableProvisioner  = &dockerProvisioner{}
	_ provision.OptionalLogsProvisioner   = &dockerProvisioner{}
	_ provision.UnitStatusProvisioner     = &dockerProvisioner{}
	_ provision.NodeProvisioner           = &dockerProvisioner{}
	_ provision.NodeRebalanceProvisioner  = &dockerProvisioner{}
	_ provision.NodeContainerProvisioner  = &dockerProvisioner{}
	_ provision.UnitFinderProvisioner     = &dockerProvisioner{}
	_ provision.AppFilterProvisioner      = &dockerProvisioner{}
	_ provision.BuilderDeploy             = &dockerProvisioner{}
	_ provision.BuilderDeployDockerClient = &dockerProvisioner{}
)

type hookHealer struct {
	p *dockerProvisioner
}

func (h hookHealer) HandleError(node *cluster.Node) time.Duration {
	return tsuruHealer.HealerInstance.HandleError(&clusterNodeWrapper{Node: node, prov: h.p})
}

func (p *dockerProvisioner) initDockerCluster() error {
	debug, _ := config.GetBool("debug")
	clusterLog.SetDebug(debug)
	clusterLog.SetLogger(log.GetStdLogger())
	var err error
	if p.storage == nil {
		p.storage, err = buildClusterStorage()
		if err != nil {
			return err
		}
	}
	if p.collectionName == "" {
		var name string
		name, err = config.GetString("docker:collection")
		if err != nil {
			if serr, ok := err.(*config.InvalidValue); ok {
				return serr
			}
			name = provisionerCollectionName
		}
		p.collectionName = name
	}
	var nodes []cluster.Node
	TotalMemoryMetadata, _ := config.GetString("docker:scheduler:total-memory-metadata")
	maxUsedMemory, _ := config.GetFloat("docker:scheduler:max-used-memory")
	p.scheduler = &segregatedScheduler{
		maxMemoryRatio:      float32(maxUsedMemory),
		TotalMemoryMetadata: TotalMemoryMetadata,
		provisioner:         p,
	}
	caPath, _ := config.GetString("docker:tls:root-path")
	p.cluster, err = cluster.New(p.scheduler, p.storage, caPath, nodes...)
	if err != nil {
		return err
	}
	p.cluster.AddHook(cluster.HookEventBeforeContainerCreate, &internalNodeContainer.ClusterHook{Provisioner: p})
	if tsuruHealer.HealerInstance != nil {
		healer := hookHealer{p: p}
		p.cluster.Healer = healer
	}
	healContainersSeconds, _ := config.GetInt("docker:healing:heal-containers-timeout")
	if healContainersSeconds > 0 {
		contHealerInst := healer.NewContainerHealer(healer.ContainerHealerArgs{
			Provisioner:         p,
			MaxUnresponsiveTime: time.Duration(healContainersSeconds) * time.Second,
			Done:                make(chan bool),
			Locker:              &appLocker{},
		})
		shutdown.Register(contHealerInst)
		go contHealerInst.RunContainerHealer()
	}
	activeMonitoring, _ := config.GetInt("docker:healing:active-monitoring-interval")
	if activeMonitoring > 0 {
		p.cluster.StartActiveMonitoring(time.Duration(activeMonitoring) * time.Second)
	}
	limitMode, _ := config.GetString("docker:limit:mode")
	if limitMode == "global" {
		p.actionLimiter = &provision.MongodbLimiter{}
	} else {
		p.actionLimiter = &provision.LocalLimiter{}
	}
	actionLimit, _ := config.GetUint("docker:limit:actions-per-host")
	if actionLimit > 0 {
		p.actionLimiter.Initialize(actionLimit)
	}
	return nil
}

func (p *dockerProvisioner) ActionLimiter() provision.ActionLimiter {
	return p.actionLimiter
}

func (p *dockerProvisioner) cloneProvisioner(ignoredContainers []container.Container) (*dockerProvisioner, error) {
	var err error
	overridenProvisioner := *p
	containerIds := make([]string, len(ignoredContainers))
	for i := range ignoredContainers {
		containerIds[i] = ignoredContainers[i].ID
	}
	overridenProvisioner.scheduler = &segregatedScheduler{
		maxMemoryRatio:      p.scheduler.maxMemoryRatio,
		TotalMemoryMetadata: p.scheduler.TotalMemoryMetadata,
		provisioner:         &overridenProvisioner,
		ignoredContainers:   containerIds,
	}
	caPath, _ := config.GetString("docker:tls:root-path")
	overridenProvisioner.cluster, err = cluster.New(overridenProvisioner.scheduler, p.storage, caPath)
	if err != nil {
		return nil, err
	}
	overridenProvisioner.cluster.Healer = p.cluster.Healer
	return &overridenProvisioner, nil
}

func (p *dockerProvisioner) stopDryMode() {
	if p.isDryMode {
		p.cluster.StopDryMode()
		coll := p.Collection()
		defer coll.Close()
		coll.DropCollection()
	}
}

func (p *dockerProvisioner) dryMode(ignoredContainers []container.Container) (*dockerProvisioner, error) {
	var err error
	overridenProvisioner := &dockerProvisioner{
		collectionName: "containers_dry_" + randomString(),
		isDryMode:      true,
		actionLimiter:  &provision.LocalLimiter{},
	}
	containerIds := make([]string, len(ignoredContainers))
	for i := range ignoredContainers {
		containerIds[i] = ignoredContainers[i].ID
	}
	overridenProvisioner.scheduler = &segregatedScheduler{
		maxMemoryRatio:      p.scheduler.maxMemoryRatio,
		TotalMemoryMetadata: p.scheduler.TotalMemoryMetadata,
		provisioner:         overridenProvisioner,
		ignoredContainers:   containerIds,
	}
	caPath, _ := config.GetString("docker:tls:root-path")
	overridenProvisioner.cluster, err = cluster.New(overridenProvisioner.scheduler, p.storage, caPath)
	if err != nil {
		return nil, err
	}
	overridenProvisioner.cluster.DryMode()
	containersToCopy, err := p.listAllContainers()
	if err != nil {
		return nil, err
	}
	coll := overridenProvisioner.Collection()
	defer coll.Close()
	toInsert := make([]interface{}, len(containersToCopy))
	for i := range containersToCopy {
		toInsert[i] = containersToCopy[i]
	}
	if len(toInsert) > 0 {
		err = coll.Insert(toInsert...)
		if err != nil {
			return nil, err
		}
	}
	return overridenProvisioner, nil
}

func (p *dockerProvisioner) Cluster() *cluster.Cluster {
	if p.cluster == nil {
		panic("nil cluster")
	}
	return p.cluster
}

func (p *dockerProvisioner) StartupMessage() (string, error) {
	nodeList, err := p.Cluster().UnfilteredNodes()
	if err != nil {
		return "", err
	}
	out := "Docker provisioner reports the following nodes:\n"
	for _, node := range nodeList {
		out += fmt.Sprintf("    Docker node: %s\n", node.Address)
	}
	if len(nodeList) == 0 {
		out += "    No Docker node available.\n"
	}
	return out, nil
}

func (p *dockerProvisioner) Initialize() error {
	err := internalNodeContainer.RegisterQueueTask(p)
	if err != nil {
		return err
	}
	return p.initDockerCluster()
}

func (p *dockerProvisioner) Provision(app provision.App) error {
	return nil
}

func (p *dockerProvisioner) Restart(a provision.App, process string, w io.Writer) error {
	containers, err := p.listContainersByProcess(a.GetName(), process)
	if err != nil {
		return err
	}
	imageID, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	if w == nil {
		w = ioutil.Discard
	}
	toAdd := make(map[string]*containersToAdd, len(containers))
	for _, c := range containers {
		if _, ok := toAdd[c.ProcessName]; !ok {
			toAdd[c.ProcessName] = &containersToAdd{Quantity: 0}
		}
		toAdd[c.ProcessName].Quantity++
		toAdd[c.ProcessName].Status = provision.StatusStarted
	}
	_, err = p.runReplaceUnitsPipeline(w, a, toAdd, containers, imageID)
	return err
}

func (p *dockerProvisioner) Start(app provision.App, process string) error {
	containers, err := p.listContainersByProcess(app.GetName(), process)
	if err != nil {
		return errors.New(fmt.Sprintf("Got error while getting app containers: %s", err))
	}
	err = runInContainers(containers, func(c *container.Container, _ chan *container.Container) error {
		startErr := c.Start(&container.StartArgs{
			Client:  p.ClusterClient(),
			Limiter: p.ActionLimiter(),
			App:     app,
		})
		if startErr != nil {
			return startErr
		}
		c.SetStatus(p.ClusterClient(), provision.StatusStarting, true)
		if info, infoErr := c.NetworkInfo(p.ClusterClient()); infoErr == nil {
			p.fixContainer(c, info)
		}
		return nil
	}, nil, true)
	return err
}

func (p *dockerProvisioner) Stop(app provision.App, process string) error {
	containers, err := p.listContainersByProcess(app.GetName(), process)
	if err != nil {
		log.Errorf("Got error while getting app containers: %s", err)
		return nil
	}
	return runInContainers(containers, func(c *container.Container, _ chan *container.Container) error {
		err := c.Stop(p.ClusterClient(), p.ActionLimiter())
		if err != nil {
			log.Errorf("Failed to stop %q: %s", app.GetName(), err)
		}
		return err
	}, nil, true)
}

func (p *dockerProvisioner) Sleep(app provision.App, process string) error {
	containers, err := p.listContainersByProcess(app.GetName(), process)
	if err != nil {
		log.Errorf("Got error while getting app containers: %s", err)
		return nil
	}
	return runInContainers(containers, func(c *container.Container, _ chan *container.Container) error {
		err := c.Sleep(p.ClusterClient(), p.ActionLimiter())
		if err != nil {
			log.Errorf("Failed to sleep %q: %s", app.GetName(), err)
		}
		return err
	}, nil, true)
}

func (p *dockerProvisioner) Rollback(a provision.App, imageID string, evt *event.Event) (string, error) {
	imageID, err := image.GetAppImageBySuffix(a.GetName(), imageID)
	if err != nil {
		return "", err
	}
	imgMetaData, err := image.GetImageMetaData(imageID)
	if err != nil {
		return "", err
	}
	if imgMetaData.DisableRollback {
		return "", fmt.Errorf("Can't Rollback image %s, reason: %s", imageID, imgMetaData.Reason)
	}
	return imageID, p.deploy(a, imageID, evt)
}

func (p *dockerProvisioner) Deploy(app provision.App, buildImageID string, evt *event.Event) (string, error) {
	if !strings.HasSuffix(buildImageID, "-builder") {
		err := p.deploy(app, buildImageID, evt)
		if err != nil {
			return "", err
		}
		return buildImageID, nil
	}
	cmds := dockercommon.DeployCmds(app)
	imageID, err := p.deployPipeline(app, buildImageID, cmds, evt)
	if err != nil {
		return "", err
	}
	err = p.deployAndClean(app, imageID, evt)
	if err != nil {
		return "", err
	}
	return imageID, nil
}

func (p *dockerProvisioner) deployAndClean(a provision.App, imageID string, evt *event.Event) error {
	err := p.deploy(a, imageID, evt)
	if err != nil {
		gc.CleanImage(a.GetName(), imageID, true)
	}
	return err
}

func (p *dockerProvisioner) deploy(a provision.App, imageID string, evt *event.Event) error {
	if err := checkCanceled(evt); err != nil {
		return err
	}
	containers, err := p.listContainersByApp(a.GetName())
	if err != nil {
		return err
	}
	imageData, err := image.GetImageMetaData(imageID)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		toAdd := make(map[string]*containersToAdd, len(imageData.Processes))
		for processName := range imageData.Processes {
			_, ok := toAdd[processName]
			if !ok {
				ct := containersToAdd{Quantity: 0}
				toAdd[processName] = &ct
			}
			toAdd[processName].Quantity++
		}
		if err = setQuota(a, toAdd); err != nil {
			return err
		}
		exposedPort := ""
		if len(imageData.ExposedPorts) > 0 {
			exposedPort = imageData.ExposedPorts[0]
		}
		_, err = p.runCreateUnitsPipeline(evt, a, toAdd, imageID, exposedPort)
	} else {
		toAdd := getContainersToAdd(imageData, containers)
		if err = setQuota(a, toAdd); err != nil {
			return err
		}
		_, err = p.runReplaceUnitsPipeline(evt, a, toAdd, containers, imageID)
	}
	if err != nil {
		err = provision.ErrUnitStartup{Err: err}
	}
	return err
}

func setQuota(app provision.App, toAdd map[string]*containersToAdd) error {
	var total int
	for _, ct := range toAdd {
		total += ct.Quantity
	}
	err := app.SetQuotaInUse(total)
	if err != nil {
		return &tsuruErrors.CompositeError{
			Base:    err,
			Message: "Cannot start application units",
		}
	}
	return nil
}

func getContainersToAdd(data image.ImageMetadata, oldContainers []container.Container) map[string]*containersToAdd {
	processMap := make(map[string]*containersToAdd, len(data.Processes))
	for name := range data.Processes {
		processMap[name] = &containersToAdd{}
	}
	minCount := 0
	for _, container := range oldContainers {
		if container.ProcessName == "" {
			minCount++
		}
		if _, ok := processMap[container.ProcessName]; ok {
			processMap[container.ProcessName].Quantity++
		}
	}
	if minCount == 0 {
		minCount = 1
	}
	for name, cont := range processMap {
		if cont.Quantity == 0 {
			processMap[name].Quantity = minCount
		}
	}
	return processMap
}

func (p *dockerProvisioner) Destroy(app provision.App) error {
	containers, err := p.listContainersByApp(app.GetName())
	if err != nil {
		log.Errorf("Failed to list app containers: %s", err)
		return err
	}
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    containers,
		writer:      ioutil.Discard,
		provisioner: p,
		appDestroy:  true,
	}
	pipeline := action.NewPipeline(
		&removeOldRoutes,
		&provisionRemoveOldUnits,
		&provisionUnbindOldUnits,
	)
	return pipeline.Execute(args)
}

func (p *dockerProvisioner) runRestartAfterHooks(cont *container.Container, w io.Writer) error {
	yamlData, err := image.GetImageTsuruYamlData(cont.Image)
	if err != nil {
		return err
	}
	if yamlData.Hooks == nil {
		return nil
	}
	cmds := yamlData.Hooks.Restart.After
	for _, cmd := range cmds {
		err := cont.Exec(p.ClusterClient(), nil, w, w, container.Pty{}, "/bin/sh", "-lc", cmd)
		if err != nil {
			return errors.Wrapf(err, "couldn't execute restart:after hook %q(%s)", cmd, cont.ShortID())
		}
	}
	return nil
}

func addContainersWithHost(args *changeUnitsPipelineArgs) ([]container.Container, error) {
	a := args.app
	w := args.writer
	var units int
	processMsg := make([]string, 0, len(args.toAdd))
	imageID := args.imageID
	for processName, v := range args.toAdd {
		units += v.Quantity
		if processName == "" {
			_, processName, _ = dockercommon.ProcessCmdForImage(processName, imageID)
		}
		processMsg = append(processMsg, fmt.Sprintf("[%s: %d]", processName, v.Quantity))
	}
	var destinationHost []string
	if args.toHost != "" {
		destinationHost = []string{args.toHost}
	}
	if w == nil {
		w = ioutil.Discard
	}
	fmt.Fprintf(w, "\n---- Starting %d new %s %s ----\n", units, pluralize("unit", units), strings.Join(processMsg, " "))
	oldContainers := make([]container.Container, 0, units)
	for processName, cont := range args.toAdd {
		for i := 0; i < cont.Quantity; i++ {
			oldContainers = append(oldContainers, container.Container{
				Container: types.Container{
					ProcessName: processName,
					Status:      cont.Status.String(),
				},
			})
		}
	}
	rollbackCallback := func(c *container.Container) {
		log.Errorf("Removing container %q due failed add units.", c.ID)
		errRem := c.Remove(args.provisioner.ClusterClient(), args.provisioner.ActionLimiter())
		if errRem != nil {
			log.Errorf("Unable to destroy container %q: %s", c.ID, errRem)
		}
	}
	var (
		createdContainers []*container.Container
		m                 sync.Mutex
	)
	err := runInContainers(oldContainers, func(c *container.Container, toRollback chan *container.Container) error {
		c, startErr := args.provisioner.start(c, a, imageID, w, args.exposedPort, destinationHost...)
		if startErr != nil {
			return startErr
		}
		toRollback <- c
		m.Lock()
		createdContainers = append(createdContainers, c)
		m.Unlock()
		fmt.Fprintf(w, " ---> Started unit %s [%s]\n", c.ShortID(), c.ProcessName)
		return nil
	}, rollbackCallback, true)
	if err != nil {
		return nil, err
	}
	result := make([]container.Container, len(createdContainers))
	i := 0
	for _, c := range createdContainers {
		result[i] = *c
		i++
	}
	return result, nil
}

func (p *dockerProvisioner) AddUnits(a provision.App, units uint, process string, w io.Writer) error {
	if a.GetDeploys() == 0 {
		return errors.New("New units can only be added after the first deployment")
	}
	if units == 0 {
		return errors.New("Cannot add 0 units")
	}
	if w == nil {
		w = ioutil.Discard
	}
	imageID, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	imageData, err := image.GetImageMetaData(imageID)
	if err != nil {
		return err
	}
	exposedPort := ""
	if len(imageData.ExposedPorts) > 0 {
		exposedPort = imageData.ExposedPorts[0]
	}
	_, err = p.runCreateUnitsPipeline(w, a, map[string]*containersToAdd{process: {Quantity: int(units)}}, imageID, exposedPort)
	return err
}

func (p *dockerProvisioner) RemoveUnits(a provision.App, units uint, processName string, w io.Writer) error {
	if a == nil {
		return errors.New("remove units: app should not be nil")
	}
	if units == 0 {
		return errors.New("cannot remove zero units")
	}
	var err error
	if w == nil {
		w = ioutil.Discard
	}
	imgID, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	_, processName, err = dockercommon.ProcessCmdForImage(processName, imgID)
	if err != nil {
		return err
	}
	containers, err := p.listContainersByProcess(a.GetName(), processName)
	if err != nil {
		return err
	}
	if len(containers) < int(units) {
		return errors.Errorf("cannot remove %d units from process %q, only %d available", units, processName, len(containers))
	}
	fmt.Fprintf(w, "\n---- Removing %d %s ----\n", units, pluralize("unit", int(units)))
	p, err = p.cloneProvisioner(nil)
	if err != nil {
		return err
	}
	toRemove := make([]container.Container, 0, units)
	for i := 0; i < int(units); i++ {
		var (
			containerID string
			cont        *container.Container
		)
		containerID, err = p.scheduler.GetRemovableContainer(a.GetName(), processName)
		if err != nil {
			return err
		}
		cont, err = p.GetContainer(containerID)
		if err != nil {
			return err
		}
		p.scheduler.ignoredContainers = append(p.scheduler.ignoredContainers, cont.ID)
		toRemove = append(toRemove, *cont)
	}
	args := changeUnitsPipelineArgs{
		app:         a,
		toRemove:    toRemove,
		writer:      w,
		provisioner: p,
	}
	pipeline := action.NewPipeline(
		&removeOldRoutes,
		&provisionRemoveOldUnits,
		&provisionUnbindOldUnits,
	)
	err = pipeline.Execute(args)
	if err != nil {
		return errors.Wrap(err, "error removing routes, units weren't removed")
	}
	return nil
}

func (p *dockerProvisioner) SetUnitStatus(unit provision.Unit, status provision.Status) error {
	cont, err := p.GetContainer(unit.ID)
	if _, ok := err.(*provision.UnitNotFoundError); ok && unit.Name != "" {
		cont, err = p.GetContainerByName(unit.Name)
	}
	if err != nil {
		return err
	}
	if cont.Status == provision.StatusBuilding.String() || cont.Status == provision.StatusAsleep.String() {
		return nil
	}
	currentStatus := cont.ExpectedStatus()
	if status == provision.StatusStopped || status == provision.StatusCreated {
		if currentStatus == provision.StatusStopped {
			status = provision.StatusStopped
		} else {
			status = provision.StatusError
		}
	} else if status == provision.StatusStarted {
		if currentStatus == provision.StatusStopped {
			status = provision.StatusError
		}
	}
	if unit.AppName != "" && cont.AppName != unit.AppName {
		return errors.New("wrong app name")
	}
	err = cont.SetStatus(p.ClusterClient(), status, true)
	if err != nil {
		return err
	}
	return p.checkContainer(cont)
}

func (p *dockerProvisioner) ExecuteCommand(opts provision.ExecOptions) error {
	if opts.Term != "" {
		opts.Cmds = append([]string{"/usr/bin/env", "TERM=" + opts.Term}, opts.Cmds...)
	}
	pty := container.Pty{
		Width:  opts.Width,
		Height: opts.Height,
		Term:   opts.Term,
	}
	if len(opts.Units) == 0 {
		imageID, err := image.AppCurrentImageName(opts.App.GetName())
		if err != nil {
			return err
		}
		return p.runCommandInContainer(imageID, opts.App, opts.Stdin, opts.Stdout, opts.Stderr, pty, opts.Cmds...)
	}
	for _, u := range opts.Units {
		cont, err := p.GetContainer(u)
		if err != nil {
			return err
		}
		if cont.AppName != opts.App.GetName() {
			return errors.Errorf("container %q does not belong to app %q", cont.ID, opts.App.GetName())
		}
		err = cont.Exec(p.ClusterClient(), opts.Stdin, opts.Stdout, opts.Stderr, pty, opts.Cmds...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *dockerProvisioner) Collection() *storage.Collection {
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(p.collectionName)
}

// GetAppFromUnitID returns app from unit id
func (p *dockerProvisioner) GetAppFromUnitID(unitID string) (provision.App, error) {
	cnt, err := p.GetContainer(unitID)
	if err != nil {
		return nil, err
	}
	a, err := app.GetByName(cnt.AppName)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (p *dockerProvisioner) Units(apps ...provision.App) ([]provision.Unit, error) {
	appNames := make([]string, len(apps))
	appNameMap := map[string]provision.App{}
	for i, a := range apps {
		appNames[i] = a.GetName()
		appNameMap[a.GetName()] = a
	}
	containers, err := p.listContainersByAppAndHost(appNames, nil)
	if err != nil {
		return nil, err
	}
	units := make([]provision.Unit, len(containers))
	for i, container := range containers {
		units[i] = container.AsUnit(appNameMap[container.AppName])
	}
	return units, nil
}

func (p *dockerProvisioner) RoutableAddresses(app provision.App) ([]url.URL, error) {
	imageID, err := image.AppCurrentImageName(app.GetName())
	if err != nil && err != image.ErrNoImagesAvailable {
		return nil, err
	}
	webProcessName, err := image.GetImageWebProcessName(imageID)
	if err != nil {
		return nil, err
	}
	containers, err := p.listContainersByApp(app.GetName())
	if err != nil {
		return nil, err
	}
	addrs := make([]url.URL, 0, len(containers))
	for _, container := range containers {
		if container.ProcessName == webProcessName && container.ValidAddr() {
			addrs = append(addrs, *container.Address())
		}
	}
	return addrs, nil
}

func (p *dockerProvisioner) RegisterUnit(a provision.App, unitId string, customData map[string]interface{}) error {
	cont, err := p.GetContainer(unitId)
	if err != nil {
		return err
	}
	if cont.Status == provision.StatusBuilding.String() {
		if cont.BuildingImage != "" && customData != nil {
			return image.SaveImageCustomData(cont.BuildingImage, customData)
		}
		return nil
	}
	err = cont.SetStatus(p.ClusterClient(), provision.StatusStarted, true)
	if err != nil {
		return err
	}
	return p.checkContainer(cont)
}

func (p *dockerProvisioner) Nodes(app provision.App) ([]cluster.Node, error) {
	poolName := app.GetPool()
	nodes, err := p.Cluster().NodesForMetadata(map[string]string{provision.PoolMetadataName: poolName})
	if err != nil {
		return nil, err
	}
	if len(nodes) > 0 {
		return nodes, nil
	}
	return nil, errors.Errorf("No nodes found with one of the following metadata: pool=%s", poolName)
}

func (p *dockerProvisioner) LogsEnabled(app provision.App) (bool, string, error) {
	const (
		logBackendsEnv      = "LOG_BACKENDS"
		logDocKeyFormat     = "LOG_%s_DOC"
		tsuruLogBackendName = "tsuru"
	)
	isBS, err := container.LogIsBS(app.GetPool())
	if err != nil {
		return false, "", err
	}
	if !isBS {
		driver, _, _ := container.LogOpts(app.GetPool())
		msg := fmt.Sprintf("Logs not available through tsuru. Enabled log driver is %q.", driver)
		return false, msg, nil
	}
	bsContainer, err := nodecontainer.LoadNodeContainer(app.GetPool(), nodecontainer.BsDefaultName)
	if err != nil {
		return false, "", err
	}
	envs := bsContainer.EnvMap()
	enabledBackends := envs[logBackendsEnv]
	if enabledBackends == "" {
		return true, "", nil
	}
	backendsList := strings.Split(enabledBackends, ",")
	for i := range backendsList {
		backendsList[i] = strings.TrimSpace(backendsList[i])
		if backendsList[i] == tsuruLogBackendName {
			return true, "", nil
		}
	}
	var docs []string
	for _, backendName := range backendsList {
		keyName := fmt.Sprintf(logDocKeyFormat, strings.ToUpper(backendName))
		backendDoc := envs[keyName]
		var docLine string
		if backendDoc == "" {
			docLine = fmt.Sprintf("* %s", backendName)
		} else {
			docLine = fmt.Sprintf("* %s: %s", backendName, backendDoc)
		}
		docs = append(docs, docLine)
	}
	fullDoc := fmt.Sprintf("Logs not available through tsuru. Enabled log backends are:\n%s",
		strings.Join(docs, "\n"))
	return false, fullDoc, nil
}

func pluralize(str string, sz int) string {
	if sz == 0 || sz > 1 {
		str = str + "s"
	}
	return str
}

func (p *dockerProvisioner) FilterAppsByUnitStatus(apps []provision.App, status []string) ([]provision.App, error) {
	if apps == nil {
		return nil, errors.Errorf("apps must be provided to FilterAppsByUnitStatus")
	}
	if status == nil {
		return make([]provision.App, 0), nil
	}
	appNames := make([]string, len(apps))
	for i, app := range apps {
		appNames[i] = app.GetName()
	}
	containers, err := p.listContainersByAppAndStatus(appNames, status)
	if err != nil {
		return nil, err
	}
	result := make([]provision.App, 0)
	for _, app := range apps {
		for _, c := range containers {
			if app.GetName() == c.AppName {
				result = append(result, app)
				break
			}
		}
	}
	return result, nil
}

var (
	_ provision.Node              = &clusterNodeWrapper{}
	_ provision.NodeHealthChecker = &clusterNodeWrapper{}
)

type clusterNodeWrapper struct {
	*cluster.Node
	prov *dockerProvisioner
}

func (n *clusterNodeWrapper) IaaSID() string {
	return n.Node.Metadata[provision.IaaSIDMetadataName]
}

func (n *clusterNodeWrapper) Address() string {
	return n.Node.Address
}

func (n *clusterNodeWrapper) Pool() string {
	return n.Node.Metadata[provision.PoolMetadataName]
}

func (n *clusterNodeWrapper) Metadata() map[string]string {
	return n.Node.CleanMetadata()
}

func (n *clusterNodeWrapper) MetadataNoPrefix() map[string]string {
	return n.Metadata()
}

func (n *clusterNodeWrapper) ExtraData() map[string]string {
	return n.Node.ExtraMetadata()
}

func (n *clusterNodeWrapper) Units() ([]provision.Unit, error) {
	if n.prov == nil {
		return nil, errors.New("no provisioner instance in node wrapper")
	}
	conts, err := n.prov.listContainersByHost(net.URLToHost(n.Address()))
	if err != nil {
		return nil, err
	}
	units := make([]provision.Unit, len(conts))
	for i, c := range conts {
		a, err := app.GetByName(c.AppName)
		if err != nil {
			return nil, err
		}
		units[i] = c.AsUnit(a)
	}
	return units, nil
}

func (n *clusterNodeWrapper) Provisioner() provision.NodeProvisioner {
	return n.prov
}

func (p *dockerProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	nodes, err := p.Cluster().UnfilteredNodes()
	if err != nil {
		return nil, err
	}
	var (
		addressSet map[string]struct{}
		result     []provision.Node
	)
	if addressFilter != nil {
		addressSet = map[string]struct{}{}
		for _, a := range addressFilter {
			addressSet[a] = struct{}{}
		}
		result = make([]provision.Node, 0, len(addressFilter))
	} else {
		result = make([]provision.Node, 0, len(nodes))
	}
	for i := range nodes {
		n := &nodes[i]
		if addressSet != nil {
			if _, ok := addressSet[n.Address]; !ok {
				continue
			}
		}
		result = append(result, &clusterNodeWrapper{Node: n, prov: p})
	}
	return result, nil
}

func (p *dockerProvisioner) ListNodesByFilter(filter *provTypes.NodeFilter) ([]provision.Node, error) {
	nodes, err := p.Cluster().UnfilteredNodesForMetadata(filter.Metadata)
	if err != nil {
		return nil, err
	}
	result := make([]provision.Node, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		result[i] = &clusterNodeWrapper{Node: n, prov: p}
	}
	return result, nil
}

func (p *dockerProvisioner) NodeForNodeData(nodeData provision.NodeStatusData) (provision.Node, error) {
	nodes, err := p.Cluster().UnfilteredNodes()
	if err != nil {
		return nil, err
	}
	nodeSet := map[string]*cluster.Node{}
	for i := range nodes {
		nodeSet[net.URLToHost(nodes[i].Address)] = &nodes[i]
	}
	containerIDs := make([]string, 0, len(nodeData.Units))
	containerNames := make([]string, 0, len(nodeData.Units))
	for _, u := range nodeData.Units {
		if u.ID != "" {
			containerIDs = append(containerIDs, u.ID)
		}
		if u.Name != "" {
			containerNames = append(containerNames, u.Name)
		}
	}
	containersForNode, err := p.listContainersWithIDOrName(containerIDs, containerNames)
	if err != nil {
		return nil, err
	}
	var chosenNode *cluster.Node
	for _, c := range containersForNode {
		n := nodeSet[c.HostAddr]
		if n != nil {
			if chosenNode != nil && chosenNode.Address != n.Address {
				return nil, errors.Errorf("containers match multiple nodes: %s and %s", chosenNode.Address, n.Address)
			}
			chosenNode = n
		}
	}
	if chosenNode != nil {
		return &clusterNodeWrapper{Node: chosenNode, prov: p}, nil
	}
	return node.FindNodeByAddrs(p, nodeData.Addrs)
}

func (p *dockerProvisioner) GetName() string {
	return provisionerName
}

func (p *dockerProvisioner) AddNode(opts provision.AddNodeOptions) error {
	if opts.Metadata == nil {
		opts.Metadata = map[string]string{}
	}
	opts.Metadata[provision.PoolMetadataName] = opts.Pool
	opts.Metadata[provision.IaaSIDMetadataName] = opts.IaaSID
	node := cluster.Node{
		Address:        opts.Address,
		Metadata:       opts.Metadata,
		CreationStatus: cluster.NodeCreationStatusPending,
		CaCert:         opts.CaCert,
		ClientCert:     opts.ClientCert,
		ClientKey:      opts.ClientKey,
	}
	err := p.Cluster().Register(node)
	if err != nil {
		return err
	}
	q, err := queue.Queue()
	if err != nil {
		return err
	}
	jobParams := monsterqueue.JobParams{"endpoint": opts.Address, "metadata": opts.Metadata}
	var job monsterqueue.Job
	if opts.WaitTO != 0 {
		job, err = q.EnqueueWait(internalNodeContainer.QueueTaskName, jobParams, opts.WaitTO)
	} else {
		_, err = q.Enqueue(internalNodeContainer.QueueTaskName, jobParams)
	}
	if err == nil && job != nil {
		_, err = job.Result()
	}
	return err
}

func (p *dockerProvisioner) UpdateNode(opts provision.UpdateNodeOptions) error {
	if opts.Metadata == nil {
		opts.Metadata = map[string]string{}
	}
	if opts.Pool != "" {
		opts.Metadata[provision.PoolMetadataName] = opts.Pool
	}
	node := cluster.Node{Address: opts.Address, Metadata: opts.Metadata}
	if opts.Disable {
		node.CreationStatus = cluster.NodeCreationStatusDisabled
	}
	if opts.Enable {
		node.CreationStatus = cluster.NodeCreationStatusCreated
	}
	_, err := mainDockerProvisioner.Cluster().UpdateNode(node)
	if err == clusterStorage.ErrNoSuchNode {
		return provision.ErrNodeNotFound
	}
	return err
}

func (p *dockerProvisioner) GetNode(address string) (provision.Node, error) {
	node, err := p.Cluster().GetNode(address)
	if err != nil {
		if err == clusterStorage.ErrNoSuchNode {
			return nil, provision.ErrNodeNotFound
		}
		return nil, err
	}
	return &clusterNodeWrapper{Node: &node, prov: p}, nil
}

func (p *dockerProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	node, err := p.Cluster().GetNode(opts.Address)
	if err != nil {
		if err == clusterStorage.ErrNoSuchNode {
			return provision.ErrNodeNotFound
		}
		return err
	}
	node.CreationStatus = cluster.NodeCreationStatusDisabled
	_, err = p.Cluster().UpdateNode(node)
	if err != nil {
		return err
	}
	if opts.Rebalance {
		err = p.rebalanceContainersByHost(net.URLToHost(opts.Address), opts.Writer)
		if err != nil {
			return err
		}
	}
	return p.Cluster().Unregister(opts.Address)
}

func (p *dockerProvisioner) UpgradeNodeContainer(name string, pool string, writer io.Writer) error {
	return internalNodeContainer.RecreateNamedContainers(p, writer, name, pool)
}

func (p *dockerProvisioner) RemoveNodeContainer(name string, pool string, writer io.Writer) error {
	return internalNodeContainer.RemoveNamedContainers(p, writer, name, pool)
}

func (p *dockerProvisioner) RebalanceNodes(opts provision.RebalanceNodesOptions) (bool, error) {
	if opts.MetadataFilter == nil {
		opts.MetadataFilter = map[string]string{}
	}
	if opts.Pool != "" {
		opts.MetadataFilter[provision.PoolMetadataName] = opts.Pool
	}
	isOnlyPool := len(opts.MetadataFilter) == 1 && opts.MetadataFilter[provision.PoolMetadataName] != ""
	if opts.Force || !isOnlyPool || len(opts.AppFilter) > 0 {
		_, err := p.rebalanceContainersByFilter(opts.Event, opts.AppFilter, opts.MetadataFilter, opts.Dry)
		return true, err
	}
	nodes, err := p.Cluster().NodesForMetadata(opts.MetadataFilter)
	if err != nil {
		return false, err
	}
	ptrNodes := make([]*cluster.Node, len(nodes))
	for i := range nodes {
		ptrNodes[i] = &nodes[i]
	}
	// No action yet, check if we need rebalance
	_, gap, err := p.containerGapInNodes(ptrNodes)
	if err != nil {
		return false, errors.Wrapf(err, "unable to obtain container gap in nodes")
	}
	buf := safe.NewBuffer(nil)
	dryProvisioner, err := p.rebalanceContainersByFilter(buf, nil, opts.MetadataFilter, true)
	if err != nil {
		return false, errors.Wrapf(err, "unable to run dry rebalance to check if rebalance is needed. log: %s", buf.String())
	}
	if dryProvisioner == nil {
		return false, nil
	}
	_, gapAfter, err := dryProvisioner.containerGapInNodes(ptrNodes)
	if err != nil {
		return false, errors.Wrap(err, "couldn't find containers from rebalanced nodes")
	}
	if math.Abs((float64)(gap-gapAfter)) > 2.0 {
		fmt.Fprintf(opts.Event, "Rebalancing as gap is %d, after rebalance gap will be %d\n", gap, gapAfter)
		_, err := p.rebalanceContainersByFilter(opts.Event, nil, opts.MetadataFilter, opts.Dry)
		return true, err
	}
	return false, nil
}
