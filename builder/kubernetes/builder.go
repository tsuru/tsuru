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
	provTypes "github.com/tsuru/tsuru/types/provision"
)

var _ builder.Builder = &kubernetesBuilder{}

type kubernetesBuilder struct{}

func init() {
	builder.Register("kubernetes", &kubernetesBuilder{})
}

func (b *kubernetesBuilder) Build(prov provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (provision.NewImageInfo, error) {
	p, ok := prov.(provision.BuilderDeployKubeClient)
	if !ok {
		return nil, errors.New("provisioner not supported")
	}
	if opts.BuildFromFile {
		return nil, errors.New("build image from Dockerfile is not yet supported")
	}
	if opts.ArchiveURL != "" {
		return nil, errors.New("build image from ArchiveURL is not yet supported by kubernetes builder")
	}
	client, err := p.GetClient(app)
	if err != nil {
		return nil, err
	}
	if opts.ImageID != "" {
		return imageBuild(client, app, opts.ImageID, evt)
	}
	if opts.Rebuild {
		var tarFile io.ReadCloser
		tarFile, err = downloadFromContainer(client, app, evt)
		if err != nil {
			return nil, err
		}
		opts.ArchiveFile = tarFile
	}
	img, err := client.BuildPod(app, evt, opts.ArchiveFile, opts.Tag)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func imageBuild(client provision.BuilderKubeClient, a provision.App, imageID string, evt *event.Event) (provision.NewImageInfo, error) {
	if !strings.Contains(imageID, ":") {
		imageID = fmt.Sprintf("%s:latest", imageID)
	}
	fmt.Fprintln(evt, "---- Pulling image to tsuru ----")
	newImage, err := image.AppNewImageName(a.GetName())
	if err != nil {
		return nil, err
	}
	inspectData, err := client.ImageTagPushAndInspect(a, evt, imageID, newImage)
	if err != nil {
		return nil, err
	}
	procfile := image.GetProcessesFromProcfile(inspectData.Procfile)
	if len(procfile) == 0 {
		fmt.Fprintln(evt, " ---> Procfile not found, using entrypoint and cmd")
		cmds := append(inspectData.Image.Config.Entrypoint, inspectData.Image.Config.Cmd...)
		if len(cmds) == 0 {
			return nil, errors.New("neither Procfile nor entrypoint and cmd set")
		}
		procfile["web"] = cmds
	}
	for k, v := range procfile {
		fmt.Fprintf(evt, " ---> Process %q found with commands: %q\n", k, v)
	}
	imageData := image.ImageMetadata{
		Name:       newImage.BaseImageName(),
		Processes:  procfile,
		CustomData: tsuruYamlToCustomData(&inspectData.TsuruYaml),
	}
	imageData.ExposedPorts = make([]string, len(inspectData.Image.Config.ExposedPorts))
	i := 0
	for k := range inspectData.Image.Config.ExposedPorts {
		imageData.ExposedPorts[i] = string(k)
		i++
	}
	err = imageData.Save()
	if err != nil {
		return nil, err
	}
	return newImage, nil
}

func tsuruYamlToCustomData(yaml *provTypes.TsuruYamlData) map[string]interface{} {
	if yaml == nil {
		return nil
	}
	return map[string]interface{}{
		"healthcheck": yaml.Healthcheck,
		"hooks":       yaml.Hooks,
		"kubernetes":  yaml.Kubernetes,
	}
}

func downloadFromContainer(client provision.BuilderKubeClient, app provision.App, evt *event.Event) (io.ReadCloser, error) {
	imageName, err := image.AppCurrentBuilderImageName(app.GetName())
	if err != nil {
		return nil, errors.Errorf("App %s image not found", app.GetName())
	}
	fmt.Fprintln(evt, "---- Downloading archive from image ----")
	archiveFile, err := client.DownloadFromContainer(app, evt, imageName)
	if err != nil {
		return nil, err
	}
	return archiveFile, nil
}
