// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var (
	_ builder.Builder         = &kubernetesBuilder{}
	_ builder.PlatformBuilder = &kubernetesBuilder{}
)

func (b *kubernetesBuilder) PlatformBuild(ctx context.Context, opts appTypes.PlatformOptions) ([]string, error) {
	return b.buildPlatform(ctx, opts)
}

func (b *kubernetesBuilder) PlatformRemove(ctx context.Context, name string) error {
	// Kubernetes already removes unused images on nodes.
	return nil
}

func (b *kubernetesBuilder) buildPlatform(ctx context.Context, opts appTypes.PlatformOptions) ([]string, error) {
	client, err := getKubeClient()
	if err != nil {
		return nil, err
	}
	return client.BuildPlatformImages(ctx, opts)
}

func getKubeClient() (provision.BuilderKubeClient, error) {
	provisioners, err := provision.Registry()
	if err != nil {
		return nil, err
	}
	var client provision.BuilderKubeClient
	multiErr := tsuruErrors.NewMultiError()
	for _, p := range provisioners {
		if provisioner, ok := p.(provision.BuilderDeployKubeClient); ok {
			client, err = provisioner.GetClient(nil)
			if err != nil {
				multiErr.Add(err)
			} else if client != nil {
				return client, nil
			}
		}
	}
	if multiErr.Len() > 0 {
		return nil, multiErr
	}
	return nil, errors.New("No Kubernetes nodes available")
}
