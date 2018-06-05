// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"io"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
)

func (p *kubernetesProvisioner) GetClient(a provision.App) (provision.BuilderKubeClient, error) {
	return &KubeClient{}, nil
}

type KubeClient struct{}

func (c *KubeClient) BuildPod(a provision.App, evt *event.Event, archiveFile io.Reader, tag string) (string, error) {
	baseImage := image.GetBuildImage(a)
	buildingImage, err := image.AppNewBuilderImageName(a.GetName(), a.GetTeamOwner(), tag)
	if err != nil {
		return "", errors.WithStack(err)
	}
	buildPodName, err := buildPodNameForApp(a, "")
	if err != nil {
		return "", err
	}
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return "", err
	}
	ns, err := client.AppNamespace(a)
	if err != nil {
		return "", err
	}
	defer cleanupPod(client, buildPodName, ns)
	params := createPodParams{
		app:               a,
		client:            client,
		podName:           buildPodName,
		sourceImage:       baseImage,
		destinationImages: []string{buildingImage},
		attachInput:       archiveFile,
		attachOutput:      evt,
		inputFile:         "/home/application/archive.tar.gz",
	}
	ctx, cancel := evt.CancelableContext(context.Background())
	err = createBuildPod(ctx, params)
	cancel()
	if err != nil {
		return "", err
	}
	return buildingImage, nil
}

func (c *KubeClient) ImageTagPushAndInspect(a provision.App, imageID, newImage string) (*docker.Image, string, *provision.TsuruYamlData, error) {
	client, err := clusterForPool(a.GetPool())
	if err != nil {
		return nil, "", nil, err
	}
	inspectData, err := imageTagAndPush(client, a, imageID, newImage)
	if err != nil {
		return nil, "", nil, err
	}
	return &inspectData.Image, inspectData.Procfile, &inspectData.TsuruYaml, nil
}
