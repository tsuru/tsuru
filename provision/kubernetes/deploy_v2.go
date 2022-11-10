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
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	apptypes "github.com/tsuru/tsuru/types/app"
	provisiontypes "github.com/tsuru/tsuru/types/provision"
	"gopkg.in/yaml.v2"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ provision.BuilderDeployV2 = (*kubernetesProvisioner)(nil)

func (p *kubernetesProvisioner) DeployV2(ctx context.Context, app provision.App, args provision.DeployV2Args) (string, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return "", err
	}

	nctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w := args.Output
	if w == nil {
		w = io.Discard
	}

	newVersion, err := servicemanager.AppVersion.NewAppVersion(nctx, apptypes.NewVersionArgs{
		App:         app,
		EventID:     args.ID,
		Description: args.Description,
	})
	if err != nil {
		return "", err
	}

	c, err := clusterForPool(nctx, app.GetPool())
	if err != nil {
		return "", err
	}

	data := make([]byte, args.ArchiveSize)
	if args.ArchiveSize > 0 {
		_, err = args.Archive.Read(data)
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		defer args.Archive.Close()
	}

	envs := make(map[string]string)
	for k, v := range app.Envs() {
		envs[k] = v.Value
	}

	baseImage, err := image.GetBuildImage(nctx, app)
	if err != nil {
		return "", err
	}

	if len(args.Image) > 0 {
		baseImage = args.Image
	}

	dstImage, err := newVersion.BaseImageName()
	if err != nil {
		return "", err
	}

	dstImages := []string{dstImage}
	if repository, tag := image.SplitImageName(dstImage); tag != "latest" {
		dstImages = append(dstImages, fmt.Sprintf("%s:latest", repository))
	}

	insecureRegistry, _ := config.GetBool("docker:registry-auth:insecure")

	bs, conn, err := c.BuildServiceClient(app.GetPool())
	if err != nil {
		return "", err
	}
	defer conn.Close()

	stream, err := bs.Build(nctx, &pb.BuildRequest{
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
		return "", err
	}

	for {
		r, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if serr, ok := status.FromError(err); ok && serr.Code() == codes.Unimplemented {
			return "", provision.ErrDeployV2NotSupported
		}

		if err != nil {
			return "", err
		}

		switch r.Data.(type) {
		case *pb.BuildResponse_TsuruConfig:
			tc := r.GetTsuruConfig()

			procfile := version.GetProcessesFromProcfile(tc.Procfile)
			if len(procfile) == 0 {
				fmt.Fprintln(w, " ---> Procfile not found, using entrypoint and cmd")
				cmds := append(tc.ImageConfig.Entrypoint, tc.ImageConfig.Cmd...)
				if len(cmds) == 0 {
					return "", errors.New("neither Procfile nor entrypoint and cmd set")
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
					return "", err
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
				return "", err
			}

		case *pb.BuildResponse_Output:
			fmt.Fprintf(w, r.GetOutput())
		}
	}

	err = newVersion.CommitBaseImage()
	if err != nil {
		return "", err
	}

	err = servicecommon.RunServicePipeline(nctx, &serviceManager{client: c, writer: w}, 0, provision.DeployArgs{
		App:              app,
		Version:          newVersion,
		Event:            args.Event,
		OverrideVersions: true,
	}, nil)
	if err != nil {
		return "", err
	}

	if err = ensureAppCustomResourceSynced(nctx, c, app); err != nil {
		return "", err
	}

	return newVersion.VersionInfo().DeployImage, nil
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
