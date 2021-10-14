// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

var _ builder.Builder = &kubernetesBuilder{}

type kubernetesBuilder struct{}

func init() {
	builder.Register("kubernetes", &kubernetesBuilder{})
}

func (b *kubernetesBuilder) Build(ctx context.Context, prov provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
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
		return imageBuild(ctx, client, app, opts, evt)
	}
	if opts.Rebuild {
		var tarFile io.ReadCloser
		tarFile, err = downloadFromContainer(ctx, client, app, evt)
		if err != nil {
			return nil, err
		}
		opts.ArchiveFile = tarFile
	}
	newVersion, err := servicemanager.AppVersion.NewAppVersion(ctx, appTypes.NewVersionArgs{
		App:            app,
		EventID:        evt.UniqueID.Hex(),
		CustomBuildTag: opts.Tag,
		Description:    opts.Message,
	})
	if err != nil {
		return nil, err
	}
	err = client.BuildPod(ctx, app, evt, opts.ArchiveFile, newVersion)
	if err != nil {
		return nil, err
	}
	err = newVersion.CommitBuildImage()
	if err != nil {
		return nil, err
	}
	return newVersion, nil
}

func imageBuild(ctx context.Context, client provision.BuilderKubeClient, a provision.App, opts *builder.BuildOpts, evt *event.Event) (appTypes.AppVersion, error) {
	imageID := opts.ImageID
	if !strings.Contains(imageID, ":") {
		imageID = fmt.Sprintf("%s:latest", imageID)
	}
	fmt.Fprintln(evt, "---- Pulling image to tsuru ----")
	newVersion, err := servicemanager.AppVersion.NewAppVersion(ctx, appTypes.NewVersionArgs{
		App:         a,
		EventID:     evt.UniqueID.Hex(),
		Description: opts.Message,
	})
	if err != nil {
		return nil, err
	}
	inspectData, err := client.ImageTagPushAndInspect(ctx, a, evt, imageID, newVersion)
	if err != nil {
		return nil, err
	}
	err = newVersion.CommitBaseImage()
	if err != nil {
		return nil, err
	}
	procfile := version.GetProcessesFromProcfile(inspectData.Procfile)
	if len(procfile) == 0 {
		fmt.Fprintln(evt, " ---> Procfile not found, using entrypoint and cmd")
		cmds := append(inspectData.Image.Config.Entrypoint, inspectData.Image.Config.Cmd...)
		if len(cmds) == 0 {
			return nil, errors.New("neither Procfile nor entrypoint and cmd set")
		}
		procfile[provision.WebProcessName] = cmds
	}
	for k, v := range procfile {
		fmt.Fprintf(evt, " ---> Process %q found with commands: %q\n", k, v)
	}
	versionData := appTypes.AddVersionDataArgs{
		Processes:    procfile,
		CustomData:   tsuruYamlToCustomData(&inspectData.TsuruYaml),
		ExposedPorts: make([]string, len(inspectData.Image.Config.ExposedPorts)),
	}
	i := 0
	for k := range inspectData.Image.Config.ExposedPorts {
		versionData.ExposedPorts[i] = string(k)
		i++
	}
	err = newVersion.AddData(versionData)
	if err != nil {
		return nil, err
	}
	return newVersion, nil
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

func downloadFromContainer(ctx context.Context, client provision.BuilderKubeClient, app provision.App, evt *event.Event) (io.ReadCloser, error) {
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(ctx, app)
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(evt, "---- Downloading archive from image ----")
	archiveFile, err := client.DownloadFromContainer(ctx, app, evt, version.VersionInfo().DeployImage)
	if err != nil {
		return nil, err
	}
	return archiveFile, nil
}
