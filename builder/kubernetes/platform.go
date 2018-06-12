// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"io"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var _ builder.Builder = &kubernetesBuilder{}

func (b *kubernetesBuilder) PlatformAdd(opts appTypes.PlatformOptions) error {
	return b.buildPlatform(opts.Name, opts.Args, opts.Output, opts.Data, opts.Ctx)
}

func (b *kubernetesBuilder) PlatformUpdate(opts appTypes.PlatformOptions) error {
	return b.buildPlatform(opts.Name, opts.Args, opts.Output, opts.Data, opts.Ctx)
}

func (b *kubernetesBuilder) PlatformRemove(name string) error {
	return nil
}

func (b *kubernetesBuilder) buildPlatform(name string, args map[string]string, w io.Writer, data []byte, ctx context.Context) error {
	client, err := getKubeClient()
	if err != nil {
		return err
	}
	inputStream := builder.CompressDockerFile(data)
	return client.BuildImage(name, inputStream, w, ctx)
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
