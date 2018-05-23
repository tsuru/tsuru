// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"io"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ provision.BuilderKubeClient = &KubeClient{}

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
	defer cleanupPod(client, buildPodName)
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

func (c *KubeClient) BuildImage(name string, image string, fileContent string, output io.Writer, ctx context.Context) error {
	buildJobName := fmt.Sprintf("%s-image-build", name)
	client, err := clusterForPool("")
	if err != nil {
		return err
	}
	args := runSinglePodArgs{
		client: client,
		name:   buildJobName,
		stdout: output,
		stderr: output,
	}
	err = createImageBuildJob(ctx, image, fileContent, args)
	if err != nil {
		return err
	}
	return nil
}

func createImageBuildJob(ctx context.Context, image string, fileContent string, args runSinglePodArgs) error {
	configMapName := fmt.Sprintf("%s-cm-dockerfile", args.name)
	configMap := &apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{
			"Dockerfile": fileContent,
		},
	}
	args.context = configMapName
	_, err := args.client.CoreV1().ConfigMaps(args.client.Namespace()).Create(configMap)
	if err != nil {
		return errors.WithStack(err)
	}
	defer args.client.CoreV1().ConfigMaps(args.client.Namespace()).Delete(configMapName, &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	args.labels = provision.ImageBuildLabels(provision.ImageBuildLabelsOpts{
		Prefix:      tsuruLabelPrefix,
		Provisioner: provisionerName,
	})
	args.image = image
	return runBuildJob(args)
}
