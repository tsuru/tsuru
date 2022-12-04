// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/tsuru/config"
	pb "github.com/tsuru/deploy-agent/pkg/build/grpc_build_v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"

	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	apptypes "github.com/tsuru/tsuru/types/app"
	provisiontypes "github.com/tsuru/tsuru/types/provision"
)

var _ provision.BuilderDeployV2 = (*kubernetesProvisioner)(nil)

func (p *kubernetesProvisioner) DeployV2(ctx context.Context, app provision.App, args provision.DeployV2Args) (string, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return "", err
	}

	if args.Output == nil {
		args.Output = io.Discard
	}

	nctx, cancel := context.WithCancel(ctx)
	defer cancel()

	/* Build and push container image */
	newVersion, err := p.buildContainerImage(nctx, app, args)
	if err != nil {
		return "", err
	}

	/* Rollout new container image to the cluster */
	if err = p.deployVersion(nctx, app, args, newVersion); err != nil {
		return "", err
	}

	return newVersion.VersionInfo().DeployImage, nil
}

func (p *kubernetesProvisioner) buildContainerImage(ctx context.Context, app provision.App, args provision.DeployV2Args) (apptypes.AppVersion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	w := args.Output

	newVersion, err := servicemanager.AppVersion.NewAppVersion(ctx, apptypes.NewVersionArgs{
		App:         app,
		EventID:     args.ID,
		Description: args.Description,
	})
	if err != nil {
		return nil, err
	}

	data := make([]byte, args.ArchiveSize)
	if args.ArchiveSize > 0 {
		_, err = args.Archive.Read(data)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		defer args.Archive.Close()
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
	if len(args.Image) > 0 {
		baseImage = args.Image
	}

	dstImage, err := newVersion.BaseImageName()
	if err != nil {
		return nil, err
	}

	dstImages := []string{dstImage}
	if repository, tag := image.SplitImageName(dstImage); tag != image.LatestTag {
		dstImages = append(dstImages, fmt.Sprintf("%s:%s", repository, image.LatestTag))
	}

	insecureRegistry, _ := config.GetBool("docker:registry-auth:insecure")

	c, err := clusterForPool(ctx, app.GetPool())
	if err != nil {
		return nil, err
	}

	bs, conn, err := c.BuildServiceClient(app.GetPool())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	stream, err := bs.Build(ctx, &pb.BuildRequest{
		App: &pb.TsuruApp{
			Name:    app.GetName(),
			EnvVars: envs,
		},
		DeployOrigin:      kindToDeployOrigin(args.Kind),
		SourceImage:       baseImage,
		DestinationImages: dstImages,
		Data:              data,
		PushOptions: &pb.PushOptions{
			InsecureRegistry: insecureRegistry,
		},
	})
	if err != nil {
		return nil, err
	}

	for {
		r, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if serr, ok := status.FromError(err); ok && serr.Code() == codes.Unimplemented {
			return nil, provision.ErrDeployV2NotSupported
		}

		if err != nil {
			return nil, err
		}

		switch r.Data.(type) {
		case *pb.BuildResponse_TsuruConfig:
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

				var err error
				customData, err = tsuruYamlToCustomData(tc.TsuruYaml)
				if err != nil {
					return nil, err
				}
			}

			var exposedPorts []string
			if tc.ImageConfig != nil {
				exposedPorts = tc.ImageConfig.ExposedPorts
			}

			err = newVersion.AddData(apptypes.AddVersionDataArgs{
				Processes:    procfile,
				CustomData:   customData,
				ExposedPorts: exposedPorts,
			})
			if err != nil {
				return nil, err
			}

		case *pb.BuildResponse_Output:
			w.Write([]byte(r.GetOutput()))
		}
	}

	if err = newVersion.CommitBaseImage(); err != nil {
		return nil, err
	}

	return newVersion, nil
}

func (p *kubernetesProvisioner) deployVersion(ctx context.Context, app provision.App, args provision.DeployV2Args, version apptypes.AppVersion) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c, err := clusterForPool(ctx, app.GetPool())
	if err != nil {
		return err
	}

	var oldVersionNumber int
	if !args.PreserveVersions {
		oldVersionNumber, err = baseVersionForApp(ctx, c, app)
		if err != nil {
			return err
		}
	}

	err = servicecommon.RunServicePipeline(ctx, &serviceManager{client: c, writer: args.Output}, oldVersionNumber, provision.DeployArgs{
		App:              app,
		Version:          version,
		Event:            args.Event,
		PreserveVersions: args.PreserveVersions,
		OverrideVersions: args.OverrideVersions,
	}, nil)
	if err != nil {
		return err
	}

	if err = ensureAppCustomResourceSynced(ctx, c, app); err != nil {
		return err
	}

	return nil
}

func kindToDeployOrigin(kind string) pb.DeployOrigin {
	switch kind {
	case "upload":
		return pb.DeployOrigin_DEPLOY_ORIGIN_SOURCE_FILES

	case "image":
		return pb.DeployOrigin_DEPLOY_ORIGIN_CONTAINER_IMAGE

	default:
		return pb.DeployOrigin_DEPLOY_ORIGIN_UNSPECIFIED
	}
}

func tsuruYamlToCustomData(str string) (map[string]any, error) {
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
