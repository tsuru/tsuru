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
)

var _ provision.BuilderDeployV2 = (*kubernetesProvisioner)(nil)

func (p *kubernetesProvisioner) DeployV2(ctx context.Context, app provision.App, args provision.DeployV2Args) (string, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return "", err
	}

	if !isDeployV2Supported(args.Kind) {
		return "", provision.ErrDeployV2NotSupported
	}

	w := args.Output
	if w == nil {
		w = io.Discard
	}

	newVersion, err := servicemanager.AppVersion.NewAppVersion(ctx, apptypes.NewVersionArgs{
		App:         app,
		EventID:     args.ID,
		Description: args.Description,
	})
	if err != nil {
		return "", err
	}

	c, err := clusterForPool(ctx, app.GetPool())
	if err != nil {
		return "", err
	}

	data := make([]byte, args.ArchiveSize)
	_, err = args.Archive.Read(data)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	defer args.Archive.Close()

	envs := make(map[string]string)
	for k, v := range app.Envs() {
		envs[k] = v.Value
	}

	baseImage, err := image.GetBuildImage(ctx, app)
	if err != nil {
		return "", err
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
		return "", err
	}

	for {
		r, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("failed to receive stream from build service: %w", err)
		}

		switch r.Data.(type) {
		case *pb.BuildResponse_TsuruConfig:
			err := newVersion.AddData(apptypes.AddVersionDataArgs{
				Processes:  version.GetProcessesFromProcfile(r.GetTsuruConfig().Procfile),
				CustomData: nil,
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

	err = servicecommon.RunServicePipeline(ctx, &serviceManager{client: c, writer: w}, 0, provision.DeployArgs{
		App:              app,
		Version:          newVersion,
		Event:            args.Event,
		OverrideVersions: true,
	}, nil)
	if err != nil {
		return "", err
	}

	if err = ensureAppCustomResourceSynced(ctx, c, app); err != nil {
		return "", err
	}

	return newVersion.VersionInfo().DeployImage, nil
}

func isDeployV2Supported(kind string) bool {
	switch kind {
	case "upload":
		return true

	default:
		return false
	}
}

func kindToDeployOrigin(kind string) pb.DeployOrigin {
	switch kind {
	case "upload":
		return pb.DeployOrigin_DEPLOY_ORIGIN_SOURCE_FILES

	default:
		return pb.DeployOrigin_DEPLOY_ORIGIN_UNSPECIFIED
	}
}
