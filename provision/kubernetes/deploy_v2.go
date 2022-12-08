// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"errors"
	"io"

	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/servicecommon"
	apptypes "github.com/tsuru/tsuru/types/app"
)

var _ provision.BuilderDeployV2 = (*kubernetesProvisioner)(nil)

func (p *kubernetesProvisioner) DeployV2(ctx context.Context, app provision.App, args provision.DeployV2Args) (string, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return "", err
	}

	if app == nil {
		return "", errors.New("app not provided")
	}

	if args.Event == nil {
		return "", errors.New("event not provided")
	}

	if args.Output == nil {
		args.Output = io.Discard
	}

	/* Build and push container image */
	b, err := builder.GetForProvisioner(p)
	if err != nil {
		return "", err
	}

	bv2, ok := b.(builder.BuilderV2)
	if !ok {
		return "", provision.ErrDeployV2NotSupported
	}

	newVersion, err := bv2.BuildV2(ctx, app, args.Event, builder.BuildOpts{
		ArchiveFile: args.Archive,
		ArchiveSize: args.ArchiveSize,
		ImageID:     args.Image,
		Message:     args.Description,
		Output:      args.Output,
	})
	if err != nil {
		return "", err
	}

	/* Rollout new container image to the cluster */
	if err = p.deployVersion(ctx, app, args, newVersion); err != nil {
		return "", err
	}

	return newVersion.VersionInfo().DeployImage, nil
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
