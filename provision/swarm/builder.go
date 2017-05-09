package swarm

import (
	"fmt"
	"io"

	"github.com/docker/docker/api/types/swarm"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

func (p *swarmProvisioner) buildImage(app provision.App, archiveFile io.ReadCloser, evt *event.Event) (string, error) {
	baseImage := image.GetBuildImage(app)
	buildingImage, err := image.AppNewImageName(app.GetName())
	if err != nil {
		return "", errors.WithStack(err)
	}
	spec, err := serviceSpecForApp(tsuruServiceOpts{
		app:        app,
		image:      baseImage,
		isDeploy:   true,
		buildImage: buildingImage,
	})
	if err != nil {
		return "", err
	}
	spec.TaskTemplate.ContainerSpec.Command = []string{"/usr/bin/tail", "-f", "/dev/null"}
	spec.TaskTemplate.RestartPolicy.Condition = swarm.RestartPolicyConditionNone
	client, err := chooseDBSwarmNode()
	if err != nil {
		return "", err
	}
	srv, err := client.CreateService(docker.CreateServiceOptions{
		ServiceSpec: *spec,
	})
	if err != nil {
		return "", errors.WithStack(err)
	}
	tasks, err := waitForTasks(client, srv.ID, swarm.TaskStateRunning)
	if err != nil {
		return "", err
	}
	client, err = clientForNode(client, tasks[0].NodeID)
	if err != nil {
		return "", err
	}
	contID := tasks[0].Status.ContainerStatus.ContainerID
	imageID, fileURI, err := dockercommon.UploadToContainer(client, contID, archiveFile)
	removeErr := client.RemoveService(docker.RemoveServiceOptions{
		ID: srv.ID,
	})
	if removeErr != nil {
		return "", errors.WithStack(removeErr)
	}
	if err != nil {
		return "", errors.WithStack(err)
	}
	opts := tsuruServiceOpts{
		app:        app,
		image:      imageID,
		isDeploy:   true,
		buildImage: buildingImage,
		constraints: []string{
			fmt.Sprintf("node.id == %s", tasks[0].NodeID),
		},
	}
	cmds := dockercommon.ArchiveDeployCmds(app, fileURI)
	srvID, task, err := runOnceCmds(client, opts, cmds, evt, evt)
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
	return buildingImage, nil
}

func (p *swarmProvisioner) Deploy(app provision.App, buildImageID string, evt *event.Event) (string, error) {
	imageID, err := p.runHookDeploy(app, buildImageID, evt)
	if err != nil {
		return "", errors.WithStack(err)
	}
	err = deployProcesses(app, imageID, nil)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return imageID, nil
}

func (p *swarmProvisioner) GetDockerClient(app provision.App) (*docker.Client, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (p *swarmProvisioner) runHookDeploy(app provision.App, imageID string, evt *event.Event) (string, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return "", err
	}
	deployImage, err := image.AppVersionedImageName(app.GetName())
	if err != nil {
		return "", err
	}
	srvID, task, err := runOnceBuildCmds(client, app, nil, imageID, deployImage, evt)
	if srvID != "" {
		defer removeServiceAndLog(client, srvID)
	}
	if err != nil {
		return "", err
	}
	_, err = commitPushBuildImage(client, deployImage, task.Status.ContainerStatus.ContainerID, app)
	if err != nil {
		return "", err
	}
	return deployImage, nil
}
