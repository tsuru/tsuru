// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"

	buildpb "github.com/tsuru/deploy-agent/pkg/build/grpc_build_v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"

	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	provisionk8s "github.com/tsuru/tsuru/provision/kubernetes"
	"github.com/tsuru/tsuru/servicemanager"
	apptypes "github.com/tsuru/tsuru/types/app"
	provisiontypes "github.com/tsuru/tsuru/types/provision"
)

var _ builder.BuilderV2 = (*kubernetesBuilder)(nil)

func (b *kubernetesBuilder) BuildV2(ctx context.Context, app provision.App, evt *event.Event, opts builder.BuildOpts) (apptypes.AppVersion, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return nil, err
	}

	if app == nil {
		return nil, errors.New("app not provided")
	}

	if evt == nil {
		return nil, errors.New("event not provided")
	}

	return b.buildContainerImage(ctx, app, evt, opts)
}

func (b *kubernetesBuilder) buildContainerImage(ctx context.Context, app provision.App, evt *event.Event, opts builder.BuildOpts) (apptypes.AppVersion, error) {
	w := opts.Output
	if w == nil {
		w = io.Discard
	}

	c, err := servicemanager.Cluster.FindByPool(ctx, "kubernetes", app.GetPool())
	if err != nil {
		return nil, err
	}

	cc, err := provisionk8s.NewClusterClient(c)
	if err != nil {
		return nil, err
	}

	bs, conn, err := cc.BuildServiceClient(app.GetPool())
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
	for k, v := range app.Envs() {
		envs[k] = v.Value
	}

	baseImage, err := image.GetBuildImage(ctx, app)
	if err != nil {
		return nil, err
	}

	// FIXME: we should only use this arg iff deploy kind is from image
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

	stream, err := bs.Build(ctx, &buildpb.BuildRequest{
		Kind: kindToBuildKind(opts),
		App: &buildpb.TsuruApp{
			Name:    app.GetName(),
			EnvVars: envs,
		},
		SourceImage:       baseImage,
		DestinationImages: dstImages,
		Data:              data,
	})
	if err != nil {
		return nil, err
	}

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
			tc := r.GetTsuruConfig()

			procfile := version.GetProcessesFromProcfile(tc.Procfile)
			if len(procfile) == 0 {
				fmt.Fprintln(w, " ---> Procfile not found, using entrypoint and cmd")
				cmds := append(tc.ImageConfig.Entrypoint, tc.ImageConfig.Cmd...)
				if len(cmds) == 0 {
					return nil, errors.New("neither Procfile nor entrypoint and cmd set")
				}

				procfile[provision.WebProcessName] = cmds
			}

			for k, v := range procfile {
				fmt.Fprintf(w, " ---> Process %q found with commands: %q\n", k, v)
			}

			var customData map[string]any
			if len(tc.TsuruYaml) > 0 {
				fmt.Fprintln(w, " ---> Tsuru config file (tsuru.yaml) found")
				// TODO: maybe pretty print Tsuru YAML on w

				customData, err = tsuruYamlStringToCustomData(tc.TsuruYaml)
				if err != nil {
					return nil, err
				}
			}

			var exposedPorts []string
			if tc.ImageConfig != nil {
				exposedPorts = tc.ImageConfig.ExposedPorts
			}

			err = appVersion.AddData(apptypes.AddVersionDataArgs{
				Processes:    procfile,
				CustomData:   customData,
				ExposedPorts: exposedPorts,
			})
			if err != nil {
				return nil, err
			}

		case *buildpb.BuildResponse_Output:
			w.Write([]byte(r.GetOutput()))
		}
	}

	if err = appVersion.CommitBaseImage(); err != nil {
		return nil, err
	}

	return appVersion, nil
}

func kindToBuildKind(opts builder.BuildOpts) buildpb.BuildKind {
	if opts.ImageID != "" {
		return buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_CONTAINER_IMAGE
	}

	if opts.ArchiveSize > 0 {
		return buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_SOURCE_UPLOAD
	}

	return buildpb.BuildKind_BUILD_KIND_UNSPECIFIED
}

func tsuruYamlStringToCustomData(str string) (map[string]any, error) {
	if len(str) == 0 {
		return nil, nil
	}

	var tsuruYaml provisiontypes.TsuruYamlData
	if err := yaml.Unmarshal([]byte(str), &tsuruYaml); err != nil {
		return nil, fmt.Errorf("failed to decode Tsuru YAML: %w", err)
	}

	return map[string]any{
		"healthcheck": tsuruYaml.Healthcheck,
		"hooks":       tsuruYaml.Hooks,
		"kubernetes":  tsuruYaml.Kubernetes,
	}, nil
}
