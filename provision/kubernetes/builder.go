// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	v1 "k8s.io/api/core/v1"
)

var _ provision.BuilderKubeClient = &KubeClient{}

func (p *kubernetesProvisioner) GetClient(a provision.App) (provision.BuilderKubeClient, error) {
	return &KubeClient{}, nil
}

type KubeClient struct{}

func (c *KubeClient) BuildPod(ctx context.Context, a provision.App, evt *event.Event, archiveFile io.Reader, version appTypes.AppVersion) error {
	baseImage, err := image.GetBuildImage(ctx, a)
	if err != nil {
		return errors.WithStack(err)
	}
	buildPodName := buildPodNameForApp(a, version)
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}
	defer cleanupPod(tsuruNet.WithoutCancel(ctx), client, buildPodName, ns)
	inputFile := "/home/application/archive.tar.gz"
	buildPlans, err := resourceRequirementsForBuildPod(ctx, a, client)
	if err != nil {
		return err
	}
	quota := v1.ResourceRequirements{}
	if plan, ok := buildPlans[buildPlanKey]; ok {
		quota = plan
	}
	buildImage, err := version.BuildImageName()
	if err != nil {
		return err
	}
	params := createPodParams{
		app:               a,
		client:            client,
		podName:           buildPodName,
		sourceImage:       baseImage,
		destinationImages: []string{buildImage},
		attachInput:       archiveFile,
		attachOutput:      evt,
		inputFile:         inputFile,
		quota:             quota,
		cmds:              dockercommon.ArchiveBuildCmds(a, "file://"+inputFile),
	}
	return createPod(ctx, params)
}

func (c *KubeClient) ImageTagPushAndInspect(ctx context.Context, a provision.App, evt *event.Event, oldImage string, version appTypes.AppVersion) (provision.InspectData, error) {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return provision.InspectData{}, err
	}
	deployPodName := deployPodNameForApp(a, version)
	labels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsBuild:     true,
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return provision.InspectData{}, err
	}
	baseImage, err := version.BaseImageName()
	if err != nil {
		return provision.InspectData{}, err
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	destImages := []string{baseImage}
	repository, tag := image.SplitImageName(baseImage)
	if tag != "latest" {
		destImages = append(destImages, fmt.Sprintf("%s:latest", repository))
	}
	err = runInspectSidecar(ctx, inspectParams{
		client:            client,
		stdout:            stdout,
		stderr:            stderr,
		eventsOutput:      evt,
		app:               a,
		sourceImage:       oldImage,
		destinationImages: destImages,
		podName:           deployPodName,
		labels:            labels,
	})
	if err != nil {
		stdoutData := stdout.String()
		stderrData := stderr.String()
		msg := "unable to pull and tag image"
		if stdoutData != "" {
			msg = fmt.Sprintf("%s: stdout: %q", msg, stdoutData)
		}
		if stderrData != "" {
			msg = fmt.Sprintf("%s: stderr: %q", msg, stderrData)
		}
		return provision.InspectData{}, errors.Errorf("%s:\n%v", msg, err)
	}
	var data provision.InspectData
	bufData := stdout.String()
	err = json.NewDecoder(stdout).Decode(&data)
	if err != nil {
		return provision.InspectData{}, errors.Wrapf(err, "invalid image inspect response: %q", bufData)
	}
	return data, err
}

func (c *KubeClient) DownloadFromContainer(ctx context.Context, app provision.App, evt *event.Event, imageName string) (io.ReadCloser, error) {
	client, err := clusterForPool(ctx, app.GetPool())
	if err != nil {
		return nil, err
	}
	reader, writer := io.Pipe()
	stderr := &bytes.Buffer{}
	go func() {
		opts := execOpts{
			client:       client,
			app:          app,
			image:        imageName,
			cmds:         []string{"cat", "/home/application/archive.tar.gz"},
			stdout:       writer,
			stderr:       stderr,
			eventsOutput: evt,
		}
		err := runIsolatedCmdPod(ctx, client, opts)
		if err != nil {
			writer.CloseWithError(errors.Wrapf(err, "error reading archive, stderr: %q", stderr.String()))
		} else {
			writer.Close()
		}
	}()
	return reader, nil
}

func (c *KubeClient) BuildPlatformImages(ctx context.Context, opts appTypes.PlatformOptions) ([]string, error) {
	regsMap := map[imgTypes.ImageRegistry]*ClusterClient{}
	err := forEachCluster(ctx, func(cli *ClusterClient) error {
		if cli.DisablePlatformBuild() {
			return nil
		}
		regsMap[cli.registry()] = cli
		return nil
	})
	if err != nil {
		return nil, err
	}

	var builtImages []string
	for reg, client := range regsMap {
		var inputStream io.Reader
		if len(opts.Data) > 0 {
			inputStream = builder.CompressDockerFile(opts.Data)
		} else if opts.RollbackVersion != 0 {
			var rollbackImg string
			rollbackImg, err = servicemanager.PlatformImage.FindImage(ctx, reg, opts.Name, fmt.Sprintf("v%d", opts.RollbackVersion))
			if err != nil {
				return builtImages, err
			}
			inputStream = builder.CompressDockerFile([]byte(fmt.Sprintf("FROM %s", rollbackImg)))
		}

		imageName, err := servicemanager.PlatformImage.NewImage(ctx, reg, opts.Name, opts.Version)
		if err != nil {
			return nil, err
		}
		images := []string{imageName}
		repo, _ := image.SplitImageName(imageName)
		for _, tag := range opts.ExtraTags {
			images = append(images, fmt.Sprintf("%s:%s", repo, tag))
		}
		err = c.buildImages(ctx, client, opts.Name, images, inputStream, opts.Output)
		if err != nil {
			return builtImages, err
		}
		builtImages = append(builtImages, imageName)
	}
	return builtImages, nil
}

func (c *KubeClient) buildImages(ctx context.Context, client *ClusterClient, name string, images []string, inputStream io.Reader, output io.Writer) error {
	fmt.Fprintf(output, "---- Building platform %s on cluster %s ----\n", name, client.Name)
	for _, img := range images {
		fmt.Fprintf(output, " ---> Destination image: %s\n", img)
	}
	fmt.Fprint(output, "---- Starting build ----\n")
	buildPodName := fmt.Sprintf("%s-image-build", name)
	defer cleanupPod(tsuruNet.WithoutCancel(ctx), client, buildPodName, client.Namespace())
	params := createPodParams{
		client:            client,
		podName:           buildPodName,
		destinationImages: images,
		inputFile:         "/data/context.tar.gz",
		attachInput:       inputStream,
		attachOutput:      output,
	}
	return createImageBuildPod(ctx, params)
}
