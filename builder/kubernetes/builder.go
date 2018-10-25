// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
)

var _ builder.Builder = &kubernetesBuilder{}

type kubernetesBuilder struct{}

func init() {
	builder.Register("kubernetes", &kubernetesBuilder{})
}

func (b *kubernetesBuilder) Build(prov provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (string, error) {
	p, ok := prov.(provision.BuilderDeployKubeClient)
	if !ok {
		return "", errors.New("provisioner not supported")
	}
	if opts.BuildFromFile {
		return "", errors.New("build image from Dockerfile is not yet supported")
	}
	if opts.ArchiveURL != "" {
		return "", errors.New("build image from ArchiveURL is not yet supported by kubernetes builder")
	}
	client, err := p.GetClient(app)
	if err != nil {
		return "", err
	}
	if opts.ImageID != "" {
		return imageBuild(client, app, opts.ImageID, evt)
	}
	if opts.Rebuild {
		var tarFile io.ReadCloser
		tarFile, err = downloadFromContainer(client, app, evt)
		if err != nil {
			return "", err
		}
		opts.ArchiveFile = tarFile
	}
	imageID, err := client.BuildPod(app, evt, opts.ArchiveFile, opts.Tag)
	if err != nil {
		return "", err
	}
	return imageID, nil
}

func imageBuild(client provision.BuilderKubeClient, a provision.App, imageID string, evt *event.Event) (string, error) {
	if !strings.Contains(imageID, ":") {
		imageID = fmt.Sprintf("%s:latest", imageID)
	}
	fmt.Fprintln(evt, "---- Pulling image to tsuru ----")
	newImage, err := image.AppNewImageName(a.GetName())
	if err != nil {
		return "", err
	}
	imageInspect, procfileRaw, tsuruYaml, err := client.ImageTagPushAndInspect(a, imageID, newImage)
	if err != nil {
		return "", err
	}
	if len(imageInspect.Config.ExposedPorts) > 1 {
		return "", errors.Errorf("too many ports exposed in Dockerfile, only one allowed: %+v", imageInspect.Config.ExposedPorts)
	}
	procfile := image.GetProcessesFromProcfile(procfileRaw)
	if len(procfile) == 0 {
		fmt.Fprintln(evt, " ---> Procfile not found, using entrypoint and cmd")
		cmds := append(imageInspect.Config.Entrypoint, imageInspect.Config.Cmd...)
		if len(cmds) == 0 {
			return "", errors.New("neither Procfile nor entrypoint and cmd set")
		}
		procfile["web"] = cmds
	}
	for k, v := range procfile {
		fmt.Fprintf(evt, " ---> Process %q found with commands: %q\n", k, v)
	}
	imageData := image.ImageMetadata{
		Name:       newImage,
		Processes:  procfile,
		CustomData: tsuruYamlToCustomData(tsuruYaml),
	}
	for k := range imageInspect.Config.ExposedPorts {
		imageData.ExposedPort = string(k)
	}
	err = imageData.Save()
	if err != nil {
		return "", err
	}
	return newImage, nil
}

func tsuruYamlToCustomData(yaml *provision.TsuruYamlData) map[string]interface{} {
	if yaml == nil {
		return nil
	}
	return map[string]interface{}{
		"healthcheck": yaml.Healthcheck,
		"hooks":       yaml.Hooks,
	}
}

func downloadFromContainer(client provision.BuilderKubeClient, app provision.App, evt *event.Event) (io.ReadCloser, error) {
	imageName, err := image.AppCurrentBuilderImageName(app.GetName())
	if err != nil {
		return nil, errors.Errorf("App %s image not found", app.GetName())
	}
	fmt.Fprintln(evt, "---- Downloading archive from image ----")
	archiveFile, err := client.DownloadFromContainer(app, imageName)
	if err != nil {
		return nil, err
	}
	return archiveFile, nil
}
