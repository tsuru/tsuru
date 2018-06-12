// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"io"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/safe"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var _ builder.Builder = &dockerBuilder{}

func (b *dockerBuilder) PlatformAdd(opts appTypes.PlatformOptions) error {
	return b.buildPlatform(opts.Name, opts.Args, opts.Output, opts.Data)
}

func (b *dockerBuilder) PlatformUpdate(opts appTypes.PlatformOptions) error {
	return b.buildPlatform(opts.Name, opts.Args, opts.Output, opts.Data)
}

func (b *dockerBuilder) PlatformRemove(name string) error {
	client, err := getDockerClient()
	if err != nil {
		return err
	}
	img, err := client.InspectImage(image.PlatformImageName(name))
	if err != nil {
		return err
	}
	err = client.RemoveImage(img.ID)
	if err != nil && err == docker.ErrNoSuchImage {
		log.Errorf("error removing image %s from Docker: no such image", name)
		return nil
	}
	return err
}

func (b *dockerBuilder) buildPlatform(name string, args map[string]string, w io.Writer, data []byte) error {
	client, err := getDockerClient()
	if err != nil {
		return err
	}
	inputStream := builder.CompressDockerFile(data)

	imageName := image.PlatformImageName(name)
	client.SetTimeout(0)
	buildOptions := docker.BuildImageOptions{
		Name:              imageName,
		Pull:              true,
		NoCache:           true,
		RmTmpContainer:    true,
		InputStream:       inputStream,
		OutputStream:      &tsuruIo.DockerErrorCheckWriter{W: w},
		InactivityTimeout: net.StreamInactivityTimeout,
		RawJSONStream:     true,
	}
	err = client.BuildImage(buildOptions)
	if err != nil {
		return err
	}
	imageName, tag := image.SplitImageName(imageName)
	var tbuf safe.Buffer
	pushOpts := docker.PushImageOptions{
		Name:              imageName,
		Tag:               tag,
		OutputStream:      &tbuf,
		InactivityTimeout: net.StreamInactivityTimeout,
	}
	err = client.PushImage(pushOpts, dockercommon.RegistryAuthConfig(imageName))
	if err != nil {
		log.Errorf("[docker] Failed to push image %q (%s): %s", name, err, tbuf.String())
		return err
	}
	return nil
}

func getDockerClient() (provision.BuilderDockerClient, error) {
	provisioners, err := provision.Registry()
	if err != nil {
		return nil, err
	}
	var client provision.BuilderDockerClient
	multiErr := tsuruErrors.NewMultiError()
	for _, p := range provisioners {
		if provisioner, ok := p.(provision.BuilderDeployDockerClient); ok {
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
	return nil, errors.New("No Docker nodes available")
}
