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
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
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
	ns, err := client.AppNamespace(a)
	if err != nil {
		return err
	}
	defer cleanupPod(client, buildPodName, ns)
	params := createPodParams{
		app:               a,
		client:            client,
		podName:           buildPodName,
		sourceImage:       baseImage,
		destinationImages: []string{version.BuildImageName()},
		attachInput:       archiveFile,
		attachOutput:      evt,
		inputFile:         "/home/application/archive.tar.gz",
	}
	return createBuildPod(ctx, params)
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
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	destImages := []string{version.BaseImageName()}
	repository, tag := image.SplitImageName(version.BaseImageName())
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

func (c *KubeClient) BuildImage(ctx context.Context, name string, images []string, inputStream io.Reader, output io.Writer) error {
	buildPodName := fmt.Sprintf("%s-image-build", name)
	client, err := clusterForPoolOrAny(ctx, "")
	if err != nil {
		return err
	}
	defer cleanupPod(client, buildPodName, client.Namespace())
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
