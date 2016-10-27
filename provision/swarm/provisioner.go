// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/docker/docker/api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

const (
	provisionerName = "swarm"
)

type swarmProvisioner struct{}

func init() {
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return &swarmProvisioner{}, nil
	})
}

func (p *swarmProvisioner) Initialize() error {
	var err error
	swarmConfig.swarmPort, err = config.GetInt("swarm:swarm-port")
	if err != nil {
		swarmConfig.swarmPort = 2377
	}
	caPath, _ := config.GetString("swarm:tls:root-path")
	if caPath != "" {
		swarmConfig.tlsConfig, err = readTLSConfig(caPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *swarmProvisioner) GetName() string {
	return provisionerName
}

func (p *swarmProvisioner) Provision(a provision.App) error {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return err
	}
	_, err = client.CreateNetwork(docker.CreateNetworkOptions{
		Name:           networkNameForApp(a),
		Driver:         "overlay",
		CheckDuplicate: true,
		IPAM: docker.IPAMOptions{
			Driver: "default",
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (p *swarmProvisioner) Destroy(a provision.App) error {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return err
	}
	multiErrors := tsuruErrors.NewMultiError()
	err = client.RemoveNetwork(networkNameForApp(a))
	if err != nil {
		multiErrors.Add(errors.WithStack(err))
	}
	processes, err := allAppProcesses(a.GetName())
	if err != nil {
		multiErrors.Add(err)
	}
	for _, p := range processes {
		name := serviceNameForApp(a, p)
		err = client.RemoveService(docker.RemoveServiceOptions{
			ID: name,
		})
		if err != nil {
			if _, notFound := err.(*docker.NoSuchService); !notFound {
				multiErrors.Add(errors.WithStack(err))
			}
		}
	}
	if multiErrors.Len() > 0 {
		return multiErrors
	}
	return nil
}

func changeUnits(a provision.App, units int, processName string, w io.Writer) error {
	if a.GetDeploys() == 0 {
		return errors.New("units can only be modified after the first deploy")
	}
	if units == 0 {
		return errors.New("cannot change 0 units")
	}
	client, err := chooseDBSwarmNode()
	if err != nil {
		return err
	}
	imageId, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	if processName == "" {
		_, processName, err = dockercommon.ProcessCmdForImage(processName, imageId)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return deployProcesses(client, a, imageId, processSpec{processName: processState{increment: units}})
}

func (p *swarmProvisioner) AddUnits(a provision.App, units uint, processName string, w io.Writer) error {
	return changeUnits(a, int(units), processName, w)
}

func (p *swarmProvisioner) RemoveUnits(a provision.App, units uint, processName string, w io.Writer) error {
	return changeUnits(a, -int(units), processName, w)
}

func (p *swarmProvisioner) SetUnitStatus(provision.Unit, provision.Status) error {
	return nil
}

func changeAppState(a provision.App, process string, state processState) error {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return err
	}
	var processes []string
	if process == "" {
		processes, err = allAppProcesses(a.GetName())
		if err != nil {
			return err
		}
	} else {
		processes = []string{process}
	}
	imgID, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return errors.WithStack(err)
	}
	spec := processSpec{}
	for _, procName := range processes {
		spec[procName] = state
	}
	return deployProcesses(client, a, imgID, spec)
}

func (p *swarmProvisioner) Restart(a provision.App, process string, w io.Writer) error {
	return changeAppState(a, process, processState{start: true, restart: true})
}

func (p *swarmProvisioner) Start(a provision.App, process string) error {
	return changeAppState(a, process, processState{start: true})
}

func (p *swarmProvisioner) Stop(a provision.App, process string) error {
	return changeAppState(a, process, processState{stop: true})
}

func allAppProcesses(appName string) ([]string, error) {
	var processes []string
	imgID, err := image.AppCurrentImageName(appName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := image.GetImageCustomData(imgID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for procName := range data.Processes {
		processes = append(processes, procName)
	}
	return processes, nil
}

var stateMap = map[swarm.TaskState]provision.Status{
	swarm.TaskStateNew:       provision.StatusCreated,
	swarm.TaskStateAllocated: provision.StatusStarting,
	swarm.TaskStatePending:   provision.StatusStarting,
	swarm.TaskStateAssigned:  provision.StatusStarting,
	swarm.TaskStateAccepted:  provision.StatusStarting,
	swarm.TaskStatePreparing: provision.StatusStarting,
	swarm.TaskStateReady:     provision.StatusStarting,
	swarm.TaskStateStarting:  provision.StatusStarting,
	swarm.TaskStateRunning:   provision.StatusStarted,
	swarm.TaskStateComplete:  provision.StatusStopped,
	swarm.TaskStateShutdown:  provision.StatusStopped,
	swarm.TaskStateFailed:    provision.StatusError,
	swarm.TaskStateRejected:  provision.StatusError,
}

func taskToUnit(task *swarm.Task, service *swarm.Service, node *swarm.Node, a provision.App) provision.Unit {
	addr := node.Spec.Labels[labelNodeDockerAddr.String()]
	host := tsuruNet.URLToHost(addr)
	var pubPort uint32
	if len(service.Endpoint.Ports) > 0 {
		pubPort = service.Endpoint.Ports[0].PublishedPort
	}
	return provision.Unit{
		ID:          task.Status.ContainerStatus.ContainerID,
		AppName:     a.GetName(),
		ProcessName: service.Spec.Annotations.Labels[labelAppProcess.String()],
		Type:        a.GetPlatform(),
		Ip:          host,
		Status:      stateMap[task.Status.State],
		Address: &url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%d", host, pubPort),
		},
	}
}

func tasksToUnits(client *docker.Client, tasks []swarm.Task) ([]provision.Unit, error) {
	nodeMap := map[string]*swarm.Node{}
	serviceMap := map[string]*swarm.Service{}
	appsMap := map[string]provision.App{}
	units := []provision.Unit{}
	for _, t := range tasks {
		if t.DesiredState == swarm.TaskStateShutdown {
			continue
		}
		if _, ok := nodeMap[t.NodeID]; !ok {
			node, err := client.InspectNode(t.NodeID)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			nodeMap[t.NodeID] = node
		}
		if _, ok := serviceMap[t.ServiceID]; !ok {
			service, err := client.InspectService(t.ServiceID)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			serviceMap[t.ServiceID] = service
		}
		service := serviceMap[t.ServiceID]
		appName := service.Spec.Annotations.Labels[labelAppName.String()]
		if _, ok := appsMap[appName]; !ok {
			a, err := app.GetByName(appName)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			appsMap[appName] = a
		}
		units = append(units, taskToUnit(&t, serviceMap[t.ServiceID], nodeMap[t.NodeID], appsMap[appName]))
	}
	return units, nil
}

func (p *swarmProvisioner) Units(app provision.App) ([]provision.Unit, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return nil, err
	}
	tasks, err := client.ListTasks(docker.ListTasksOptions{
		Filters: map[string][]string{
			"label": {fmt.Sprintf("%s=%s", labelAppName, app.GetName())},
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return tasksToUnits(client, tasks)
}

func (p *swarmProvisioner) RoutableUnits(app provision.App) ([]provision.Unit, error) {
	imgID, err := image.AppCurrentImageName(app.GetName())
	if err != nil && err != image.ErrNoImagesAvailable {
		return nil, err
	}
	webProcessName, err := image.GetImageWebProcessName(imgID)
	if err != nil {
		return nil, err
	}
	units, err := p.Units(app)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(units); i++ {
		if units[i].ProcessName != webProcessName {
			units = append(units[:i], units[i+1:]...)
			i--
		}
	}
	return units, nil
}

func findTaskFromContainer(tasks []swarm.Task, contID string) *swarm.Task {
	for i, t := range tasks {
		if strings.HasPrefix(t.Status.ContainerStatus.ContainerID, contID) {
			return &tasks[i]
		}
	}
	return nil
}

func bindUnit(client *docker.Client, unit *provision.Unit) error {
	a, err := app.GetByName(unit.AppName)
	if err != nil {
		return errors.WithStack(err)
	}
	err = a.BindUnit(unit)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (p *swarmProvisioner) RegisterUnit(unit provision.Unit, customData map[string]interface{}) error {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return err
	}
	err = bindUnit(client, &unit)
	if err != nil {
		return err
	}
	if customData == nil {
		return nil
	}
	tasks, err := client.ListTasks(docker.ListTasksOptions{
		Filters: map[string][]string{
			"label": {labelServiceDeploy.String() + "=true"},
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	task := findTaskFromContainer(tasks, unit.ID)
	if task == nil {
		return nil
	}
	buildingImage := task.Spec.ContainerSpec.Labels[labelServiceBuildImage.String()]
	if buildingImage == "" {
		return errors.Errorf("invalid build image label for build task: %#v", task)
	}
	return image.SaveImageCustomData(buildingImage, customData)
}

func (p *swarmProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		if errors.Cause(err) == errNoSwarmNode {
			return nil, nil
		}
		return nil, err
	}
	nodes, err := client.ListNodes(docker.ListNodesOptions{})
	if err != nil {
		return nil, err
	}
	var filterMap map[string]struct{}
	if len(addressFilter) > 0 {
		filterMap = map[string]struct{}{}
		for _, addr := range addressFilter {
			filterMap[tsuruNet.URLToHost(addr)] = struct{}{}
		}
	}
	nodeList := make([]provision.Node, 0, len(nodes))
	for i := range nodes {
		wrapped := &swarmNodeWrapper{Node: &nodes[i], provisioner: p}
		toAdd := true
		if filterMap != nil {
			_, toAdd = filterMap[tsuruNet.URLToHost(wrapped.Address())]
		}
		if toAdd {
			nodeList = append(nodeList, wrapped)
		}
	}
	return nodeList, nil
}

func (p *swarmProvisioner) GetNode(address string) (provision.Node, error) {
	nodes, err := p.ListNodes([]string{address})
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, provision.ErrNodeNotFound
	}
	return nodes[0], nil
}

func (p *swarmProvisioner) AddNode(opts provision.AddNodeOptions) error {
	existingClient, err := chooseDBSwarmNode()
	if err != nil && errors.Cause(err) != errNoSwarmNode {
		return err
	}
	newClient, err := newClient(opts.Address)
	if err != nil {
		return err
	}
	if existingClient == nil {
		err = initSwarm(newClient, opts.Address)
	} else {
		err = joinSwarm(existingClient, newClient, opts.Address)
	}
	if err != nil {
		return err
	}
	dockerInfo, err := newClient.Info()
	if err != nil {
		return errors.WithStack(err)
	}
	nodeData, err := newClient.InspectNode(dockerInfo.Swarm.NodeID)
	if err != nil {
		return errors.WithStack(err)
	}
	nodeData.Spec.Annotations.Labels = map[string]string{
		labelNodeDockerAddr.String(): opts.Address,
	}
	for k, v := range opts.Metadata {
		nodeData.Spec.Annotations.Labels[k] = v
	}
	err = newClient.UpdateNode(dockerInfo.Swarm.NodeID, docker.UpdateNodeOptions{
		Version:  nodeData.Version.Index,
		NodeSpec: nodeData.Spec,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return updateDBSwarmNodes(newClient)
}

func (p *swarmProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	node, err := p.GetNode(opts.Address)
	if err != nil {
		return err
	}
	client, err := chooseDBSwarmNode()
	if err != nil {
		return err
	}
	swarmNode := node.(*swarmNodeWrapper).Node
	if opts.Rebalance {
		swarmNode.Spec.Availability = swarm.NodeAvailabilityDrain
		err = client.UpdateNode(swarmNode.ID, docker.UpdateNodeOptions{
			NodeSpec: swarmNode.Spec,
			Version:  swarmNode.Version.Index,
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	err = client.RemoveNode(docker.RemoveNodeOptions{
		ID:    swarmNode.ID,
		Force: true,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return updateDBSwarmNodes(client)
}

func (p *swarmProvisioner) UpdateNode(opts provision.UpdateNodeOptions) error {
	node, err := p.GetNode(opts.Address)
	if err != nil {
		return err
	}
	swarmNode := node.(*swarmNodeWrapper).Node
	if opts.Disable {
		swarmNode.Spec.Availability = swarm.NodeAvailabilityPause
	} else if opts.Enable {
		swarmNode.Spec.Availability = swarm.NodeAvailabilityActive
	}
	for k, v := range opts.Metadata {
		if v == "" {
			delete(swarmNode.Spec.Annotations.Labels, k)
		} else {
			swarmNode.Spec.Annotations.Labels[k] = v
		}
	}
	client, err := chooseDBSwarmNode()
	if err != nil {
		return err
	}
	err = client.UpdateNode(swarmNode.ID, docker.UpdateNodeOptions{
		NodeSpec: swarmNode.Spec,
		Version:  swarmNode.Version.Index,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (p *swarmProvisioner) ArchiveDeploy(app provision.App, archiveURL string, evt *event.Event) (imgID string, err error) {
	baseImage := image.GetBuildImage(app)
	buildingImage, err := image.AppNewImageName(app.GetName())
	if err != nil {
		return "", errors.WithStack(err)
	}
	cmds := dockercommon.ArchiveDeployCmds(app, archiveURL)
	client, err := chooseDBSwarmNode()
	if err != nil {
		return "", err
	}
	srvID, task, err := runOnceBuildCmds(client, app, cmds, baseImage, buildingImage, evt)
	if srvID != "" {
		defer removeServiceAndLog(client, srvID)
	}
	if err != nil {
		return "", err
	}
	_, err = commitPushBuildImage(client, buildingImage, task.Status.ContainerStatus.ContainerID, app)
	if err != nil {
		return "", err
	}
	err = deployProcesses(client, app, buildingImage, nil)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return buildingImage, nil
}

func (p *swarmProvisioner) ImageDeploy(a provision.App, imgID string, evt *event.Event) (string, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return "", err
	}
	if !strings.Contains(imgID, ":") {
		imgID = fmt.Sprintf("%s:latest", imgID)
	}
	newImage, err := image.AppNewImageName(a.GetName())
	if err != nil {
		return "", errors.WithStack(err)
	}
	fmt.Fprintln(evt, "---- Pulling image to tsuru ----")
	var buf bytes.Buffer
	cmds := []string{"/bin/bash", "-c", "cat /home/application/current/Procfile || cat /app/user/Procfile || cat /Procfile"}
	srvID, task, err := runOnceBuildCmds(client, a, cmds, imgID, newImage, &buf)
	if srvID != "" {
		defer removeServiceAndLog(client, srvID)
	}
	if err != nil {
		return "", err
	}
	client, err = clientForNode(client, task.NodeID)
	if err != nil {
		return "", err
	}
	procfileData := buf.String()
	procfile := image.GetProcessesFromProcfile(procfileData)
	imageInspect, err := client.InspectImage(imgID)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if len(procfile) == 0 {
		fmt.Fprintln(evt, "  ---> Procfile not found, trying to get entrypoint")
		if len(imageInspect.Config.Entrypoint) == 0 {
			return "", errors.New("no procfile or entrypoint found in image")
		}
		webProcess := imageInspect.Config.Entrypoint[0]
		for _, c := range imageInspect.Config.Entrypoint[1:] {
			webProcess += fmt.Sprintf(" %q", c)
		}
		procfile["web"] = webProcess
	}
	for k, v := range procfile {
		fmt.Fprintf(evt, "  ---> Process %s found with command: %v\n", k, v)
	}
	imageInfo := strings.Split(newImage, ":")
	repo, tag := strings.Join(imageInfo[:len(imageInfo)-1], ":"), imageInfo[len(imageInfo)-1]
	err = client.TagImage(imgID, docker.TagImageOptions{Repo: repo, Tag: tag, Force: true})
	if err != nil {
		return "", errors.WithStack(err)
	}
	err = pushImage(client, repo, tag)
	if err != nil {
		return "", err
	}
	imageData := image.CreateImageMetadata(newImage, procfile)
	if len(imageInspect.Config.ExposedPorts) > 1 {
		return "", errors.New("Too many ports. You should especify which one you want to.")
	}
	for k := range imageInspect.Config.ExposedPorts {
		imageData.CustomData["exposedPort"] = string(k)
	}
	err = image.SaveImageCustomData(newImage, imageData.CustomData)
	if err != nil {
		return "", errors.WithStack(err)
	}
	a.SetUpdatePlatform(true)
	err = deployProcesses(client, a, newImage, nil)
	if err != nil {
		return "", err
	}
	return newImage, nil
}

func deployProcesses(client *docker.Client, a provision.App, newImg string, updateSpec processSpec) error {
	curImg, err := image.AppCurrentImageName(a.GetName())
	if err != nil {
		return err
	}
	currentImageData, err := image.GetImageCustomData(curImg)
	if err != nil {
		return err
	}
	currentSpec := processSpec{}
	for p := range currentImageData.Processes {
		currentSpec[p] = processState{}
	}
	newImageData, err := image.GetImageCustomData(newImg)
	if err != nil {
		return err
	}
	if len(newImageData.Processes) == 0 {
		return errors.Errorf("no process information found deploying image %q", newImg)
	}
	newSpec := processSpec{}
	for p := range newImageData.Processes {
		newSpec[p] = processState{start: true}
		if updateSpec != nil {
			newSpec[p] = updateSpec[p]
		}
	}
	pipeline := action.NewPipeline(
		updateServices,
		updateImageInDB,
		removeOldServices,
	)
	return pipeline.Execute(&pipelineArgs{
		client:           client,
		app:              a,
		newImage:         newImg,
		newImageSpec:     newSpec,
		currentImage:     curImg,
		currentImageSpec: currentSpec,
	})
}

func removeService(client *docker.Client, a provision.App, process string) error {
	srvName := serviceNameForApp(a, process)
	err := client.RemoveService(docker.RemoveServiceOptions{ID: srvName})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func deploy(client *docker.Client, a provision.App, process string, pState processState, imgID string) error {
	srvName := serviceNameForApp(a, process)
	srv, err := client.InspectService(srvName)
	if err != nil {
		if _, isNotFound := err.(*docker.NoSuchService); !isNotFound {
			return errors.WithStack(err)
		}
	}
	var baseSpec *swarm.ServiceSpec
	if srv != nil {
		baseSpec = &srv.Spec
	}
	spec, err := serviceSpecForApp(tsuruServiceOpts{
		app:          a,
		process:      process,
		image:        imgID,
		baseSpec:     baseSpec,
		processState: pState,
	})
	if err != nil {
		return err
	}
	if srv == nil {
		_, err = client.CreateService(docker.CreateServiceOptions{
			ServiceSpec: *spec,
		})
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		srv.Spec = *spec
		err = client.UpdateService(srv.ID, docker.UpdateServiceOptions{
			Version:     srv.Version.Index,
			ServiceSpec: srv.Spec,
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func runOnceBuildCmds(client *docker.Client, a provision.App, cmds []string, imgID, buildingImage string, w io.Writer) (string, *swarm.Task, error) {
	spec, err := serviceSpecForApp(tsuruServiceOpts{
		app:        a,
		image:      imgID,
		isDeploy:   true,
		buildImage: buildingImage,
	})
	if err != nil {
		return "", nil, err
	}
	spec.TaskTemplate.ContainerSpec.Command = cmds
	spec.TaskTemplate.RestartPolicy.Condition = swarm.RestartPolicyConditionNone
	srv, err := client.CreateService(docker.CreateServiceOptions{
		ServiceSpec: *spec,
	})
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	createdID := srv.ID
	tasks, err := waitForTasks(client, createdID, swarm.TaskStateShutdown)
	if err != nil {
		return createdID, nil, err
	}
	client, err = clientForNode(client, tasks[0].NodeID)
	if err != nil {
		return createdID, nil, err
	}
	contID := tasks[0].Status.ContainerStatus.ContainerID
	attachOpts := docker.AttachToContainerOptions{
		Container:    contID,
		OutputStream: w,
		ErrorStream:  w,
		Logs:         true,
		Stdout:       true,
		Stderr:       true,
		Stream:       true,
	}
	exitCode, err := safeAttachWaitContainer(client, attachOpts)
	if err != nil {
		return createdID, nil, err
	}
	if exitCode != 0 {
		return createdID, nil, errors.Errorf("unexpected result code for build container: %d", exitCode)
	}
	return createdID, &tasks[0], nil
}
