// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	buildpb "github.com/tsuru/deploy-agent/pkg/build/grpc_build_v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/yaml"

	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	provisionk8s "github.com/tsuru/tsuru/provision/kubernetes"
	"github.com/tsuru/tsuru/servicemanager"
	apptypes "github.com/tsuru/tsuru/types/app"
	imagetypes "github.com/tsuru/tsuru/types/app/image"
	jobTypes "github.com/tsuru/tsuru/types/job"
	provisiontypes "github.com/tsuru/tsuru/types/provision"
)

var (
	_ builder.Builder         = &kubernetesBuilder{}
	_ builder.PlatformBuilder = &kubernetesBuilder{}
)

type kubernetesBuilder struct{}

func init() {
	builder.Register("kubernetes", &kubernetesBuilder{})
}

func (b *kubernetesBuilder) Build(ctx context.Context, app *apptypes.App, evt *event.Event, opts builder.BuildOpts) (apptypes.AppVersion, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return nil, err
	}

	if app == nil {
		return nil, errors.New("app not provided")
	}

	if evt == nil {
		return nil, errors.New("event not provided")
	}

	if opts.Rebuild {
		return nil, errors.New("app rebuild is deprecated")
	}

	if opts.ArchiveURL != "" { // build w/ external archive (ideal for Terraform)
		f, size, err := builder.DownloadArchiveFromURL(ctx, opts.ArchiveURL)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		opts.ArchiveFile = f
		opts.ArchiveSize = int64(size)
	}

	return b.buildContainerImage(ctx, app, evt, opts)
}

func (b *kubernetesBuilder) BuildJob(ctx context.Context, job *jobTypes.Job, opts builder.BuildOpts) (string, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return "", err
	}
	if job == nil {
		return "", errors.New("job not provided")
	}
	w := opts.Output
	if w == nil {
		w = io.Discard
	}
	pool := job.GetPool()
	c, err := servicemanager.Cluster.FindByPool(ctx, "kubernetes", pool)
	if err != nil {
		return "", err
	}
	cc, err := provisionk8s.NewClusterClient(c)
	if err != nil {
		return "", err
	}
	bs, conn, err := cc.BuildServiceClient(pool)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	data := make([]byte, opts.ArchiveSize)
	if opts.ArchiveSize > 0 {
		_, err = opts.ArchiveFile.Read(data)
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
	}

	baseImage := opts.ImageID
	dstImage, err := servicemanager.Job.BaseImageName(ctx, job)
	if err != nil {
		return "", err
	}
	dstImages := make([]string, 0, 2)
	dstImages = append(dstImages, dstImage)

	jobEnvVars := make(map[string]string)
	for k, v := range servicemanager.Job.GetEnvs(ctx, job) {
		jobEnvVars[k] = v.Value
	}

	req := &buildpb.BuildRequest{
		Kind: kindToJobBuildKind(opts),
		Job: &buildpb.TsuruJob{
			Name:    job.GetName(),
			EnvVars: jobEnvVars,
		},
		SourceImage:       baseImage,
		DestinationImages: dstImages,
		PushOptions:       &buildpb.PushOptions{InsecureRegistry: cc.InsecureRegistry()},
		Data:              data,
		Containerfile:     opts.Dockerfile,
	}

	_, err = callBuildService(ctx, bs, req, w)
	if err != nil {
		return "", err
	}
	return dstImage, nil
}

func (b *kubernetesBuilder) PlatformBuild(ctx context.Context, opts apptypes.PlatformOptions) ([]string, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return nil, err
	}

	w := opts.Output
	if w == nil {
		w = io.Discard
	}

	clusters, err := servicemanager.Cluster.List(ctx)
	if err != nil {
		return nil, err
	}

	var images []string

	registries := make(map[imagetypes.ImageRegistry]struct{})

	for _, c := range clusters {
		cc, err := provisionk8s.NewClusterClient(&c)
		if err != nil {
			return nil, err
		}

		bc, conn, err := cc.BuildServiceClient("") // requires the configuration to be set for all pools
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		if cc.DisablePlatformBuild() {
			fmt.Fprintf(w, "Skipping platform build on %s cluster: disabled to platform builds\n", c.Name)
			continue
		}

		registry := cc.Registry()
		if registry == imagetypes.EmptyImageRegistry {
			fmt.Fprintf(w, "Skipping platform build on %s cluster: no registry found in cluster configs\n", c.Name)
			continue
		}

		if _, found := registries[registry]; found {
			continue // already done
		}

		registries[registry] = struct{}{}

		fmt.Fprintf(w, "---- Building platform %s on cluster %s ----\n", opts.Name, c.Name)

		imgs, err := b.buildPlatformImage(ctx, bc, registry, cc.InsecureRegistry(), opts)
		if err != nil {
			return nil, err
		}

		images = append(images, imgs...)
	}

	if len(images) == 0 {
		return nil, errors.New("no kubernetes nodes available")
	}

	return images, nil
}

func (b *kubernetesBuilder) buildPlatformImage(ctx context.Context, bc buildpb.BuildClient, registry imagetypes.ImageRegistry, insecureRegistry bool, opts apptypes.PlatformOptions) ([]string, error) {
	w := opts.Output
	if w == nil {
		w = io.Discard
	}

	var containerfile string
	if opts.RollbackVersion > 0 {
		return nil, errors.New("rollback not implemented")
	}

	if len(opts.Data) > 0 {
		containerfile = string(opts.Data)
	}

	img, err := servicemanager.PlatformImage.NewImage(ctx, registry, opts.Name, opts.Version)
	if err != nil {
		return nil, err
	}

	images := make([]string, 0, 1+len(opts.ExtraTags))
	images = append(images, img)

	repo, _ := image.SplitImageName(img)
	for _, tag := range opts.ExtraTags {
		images = append(images, fmt.Sprintf("%s:%s", repo, tag))
	}

	for _, img := range images {
		fmt.Fprintf(w, " ---> Destination image: %s\n", img)
	}

	fmt.Fprintln(w, "---- Starting build ----")

	req := &buildpb.BuildRequest{
		Kind:              buildpb.BuildKind_BUILD_KIND_PLATFORM_WITH_CONTAINER_FILE,
		Platform:          &buildpb.TsuruPlatform{Name: opts.Name},
		Containerfile:     containerfile,
		DestinationImages: images,
		PushOptions:       &buildpb.PushOptions{InsecureRegistry: insecureRegistry},
	}

	_, err = callBuildService(ctx, bc, req, w)
	if err != nil {
		return nil, err
	}

	return images, nil
}

func (b *kubernetesBuilder) buildContainerImage(ctx context.Context, app *apptypes.App, evt *event.Event, opts builder.BuildOpts) (apptypes.AppVersion, error) {
	w := opts.Output
	if w == nil {
		w = io.Discard
	}

	c, err := servicemanager.Cluster.FindByPool(ctx, "kubernetes", app.Pool)
	if err != nil {
		return nil, err
	}

	cc, err := provisionk8s.NewClusterClient(c)
	if err != nil {
		return nil, err
	}

	bs, conn, err := cc.BuildServiceClient(app.Pool)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	appVersion, err := servicemanager.AppVersion.NewAppVersion(ctx, apptypes.NewVersionArgs{
		App:         app,
		EventID:     evt.UniqueID.Hex(),
		Description: opts.Message,
	})
	if err != nil {
		return nil, err
	}

	data := make([]byte, opts.ArchiveSize)
	if opts.ArchiveSize > 0 {
		_, err = opts.ArchiveFile.Read(data)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	}

	envs := make(map[string]string)
	for k, v := range provision.EnvsForApp(app) {
		envs[k] = v.Value
	}

	baseImage, err := image.GetBuildImage(ctx, app)
	if err != nil {
		return nil, err
	}

	// FIXME: we should only use this arg if deploy kind is from image
	if opts.ImageID != "" {
		baseImage = opts.ImageID
	}

	dstImage, err := appVersion.BaseImageName()
	if err != nil {
		return nil, err
	}

	dstImages := make([]string, 0, 2)
	dstImages = append(dstImages, dstImage)

	if opts.Tag == "" {
		opts.Tag = image.LatestTag
	}

	if repository, tag := image.SplitImageName(dstImage); tag != opts.Tag {
		dstImages = append(dstImages, fmt.Sprintf("%s:%s", repository, opts.Tag))
	}

	req := &buildpb.BuildRequest{
		Kind: kindToBuildKind(opts),
		App: &buildpb.TsuruApp{
			Name:    app.Name,
			Team:    app.TeamOwner,
			EnvVars: envs,
		},
		SourceImage:       baseImage,
		DestinationImages: dstImages,
		PushOptions:       &buildpb.PushOptions{InsecureRegistry: cc.InsecureRegistry()},
		Data:              data,
		Containerfile:     opts.Dockerfile,
	}

	tc, err := callBuildService(ctx, bs, req, w)
	if err != nil {
		return nil, err
	}

	if tc != nil {
		var processes map[string][]string
		var customData map[string]any
		var tsuruYamlData provisiontypes.TsuruYamlData
		if len(tc.TsuruYaml) > 0 {
			fmt.Fprintln(w, " ---> Tsuru config file (tsuru.yaml) found")
			fmt.Fprintln(w, tc.TsuruYaml)
			tsuruYamlData, err = parseTsuruYaml(tc.TsuruYaml)
			if err != nil {
				return nil, err
			}
			customData = tsuruYamlDataToCustomData(tsuruYamlData)
		}
		// Check if it uses new `processes` on YML
		if len(tsuruYamlData.Processes) > 0 {
			// If it uses, try to get processes and commands from YML
			processes = version.GetProcessesFromYamlProcess(tsuruYamlData.Processes)
		} else {
			// If it does not uses new `processes` on YML, use current implementation
			processes = version.GetProcessesFromProcfile(tc.Procfile)
		}

		// Default to web process name and entrypoint and cmd from container
		if len(processes) == 0 {
			ic := tc.ImageConfig
			if ic == nil {
				ic = new(buildpb.ContainerImageConfig) // covering to avoid panic
			}

			fmt.Fprintln(w, " ---> Procfile not found, using entrypoint and cmd")
			cmds := append(ic.Entrypoint, ic.Cmd...)
			if len(cmds) == 0 {
				return nil, errors.New("neither Procfile nor entrypoint and cmd set")
			}

			processes[provision.WebProcessName] = cmds
		}

		for k, v := range processes {
			fmt.Fprintf(w, " ---> Process %q found with commands: %q\n", k, v)
		}

		var exposedPorts []string
		if tc.ImageConfig != nil {
			exposedPorts = tc.ImageConfig.ExposedPorts
		}

		err = appVersion.AddData(apptypes.AddVersionDataArgs{
			Processes:    processes,
			CustomData:   customData,
			ExposedPorts: exposedPorts,
		})
		if err != nil {
			return nil, err
		}
	}

	if err = appVersion.CommitBaseImage(); err != nil {
		return nil, err
	}

	return appVersion, nil
}

func callBuildService(ctx context.Context, bc buildpb.BuildClient, req *buildpb.BuildRequest, w io.Writer) (*buildpb.TsuruConfig, error) {
	stream, err := bc.Build(ctx, req)
	if err != nil {
		return nil, err
	}

	var once sync.Once
	var tc *buildpb.TsuruConfig

	for {
		var r *buildpb.BuildResponse
		r, err = stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if serr, ok := status.FromError(err); ok && serr.Code() == codes.Unimplemented {
			return nil, builder.ErrBuildV2NotSupported
		}

		if err != nil {
			return nil, err
		}

		switch r.Data.(type) {
		case *buildpb.BuildResponse_TsuruConfig:
			once.Do(func() { tc = r.GetTsuruConfig() })

		case *buildpb.BuildResponse_Output:
			w.Write([]byte(r.GetOutput()))
		}
	}

	return tc, nil
}

func kindToBuildKind(opts builder.BuildOpts) buildpb.BuildKind {
	if opts.ImageID != "" {
		return buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_CONTAINER_IMAGE
	}

	if opts.Dockerfile != "" {
		return buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_CONTAINER_FILE
	}

	if opts.ArchiveSize > 0 {
		return buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_SOURCE_UPLOAD
	}

	return buildpb.BuildKind_BUILD_KIND_UNSPECIFIED
}

func kindToJobBuildKind(opts builder.BuildOpts) buildpb.BuildKind {
	if opts.ImageID != "" {
		return buildpb.BuildKind_BUILD_KIND_JOB_DEPLOY_WITH_CONTAINER_IMAGE
	}

	if opts.Dockerfile != "" {
		return buildpb.BuildKind_BUILD_KIND_JOB_DEPLOY_WITH_CONTAINER_FILE
	}

	return buildpb.BuildKind_BUILD_KIND_UNSPECIFIED
}

func parseTsuruYaml(str string) (provisiontypes.TsuruYamlData, error) {
	var tsuruYaml provisiontypes.TsuruYamlData
	// NOTE(nettoclaudio): we must use the "sigs.k8s.io/yaml" package to
	// decode the YAML from app since we need some functions of JSON decoder
	// as well - namely parse field names based on JSON struct tags.
	if err := yaml.Unmarshal([]byte(str), &tsuruYaml); err != nil {
		return provisiontypes.TsuruYamlData{}, fmt.Errorf("failed to decode Tsuru YAML: %w", err)
	}
	return tsuruYaml, nil
}

func tsuruYamlDataToCustomData(tsuruYaml provisiontypes.TsuruYamlData) map[string]any {
	return map[string]any{
		"healthcheck": tsuruYaml.Healthcheck,
		"hooks":       tsuruYaml.Hooks,
		"kubernetes":  tsuruYaml.Kubernetes,
		"processes":   tsuruYaml.Processes,
	}
}
