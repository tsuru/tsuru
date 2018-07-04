// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"

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
	return b.buildPlatform(opts)
}

func (b *dockerBuilder) PlatformUpdate(opts appTypes.PlatformOptions) error {
	return b.buildPlatform(opts)
}

func (b *dockerBuilder) PlatformRemove(name string) error {
	client, err := getDockerClient()
	if err != nil {
		return err
	}
	imgs, err := image.PlatformListImages(name)
	if err != nil {
		return err
	}
	for _, imgName := range imgs {
		img, err := client.InspectImage(imgName)
		if err == nil {
			err = client.RemoveImage(img.ID)
			if err == nil {
				continue
			}
		}
		log.Errorf("error removing image %s from Docker: %s", imgName, err)
	}
	return nil
}

func (b *dockerBuilder) buildPlatform(opts appTypes.PlatformOptions) error {
	client, err := getDockerClient()
	if err != nil {
		return err
	}
	inputStream := builder.CompressDockerFile(opts.Data)
	client.SetTimeout(0)
	buildOptions := docker.BuildImageOptions{
		Name:              opts.ImageName,
		Pull:              true,
		NoCache:           true,
		RmTmpContainer:    true,
		InputStream:       inputStream,
		OutputStream:      &tsuruIo.DockerErrorCheckWriter{W: opts.Output},
		InactivityTimeout: net.StreamInactivityTimeout,
		RawJSONStream:     true,
	}
	err = client.BuildImage(buildOptions)
	if err != nil {
		return err
	}
	imageName, tag := image.SplitImageName(opts.ImageName)
	var buf safe.Buffer
	pushOpts := docker.PushImageOptions{
		Name:              imageName,
		Tag:               tag,
		OutputStream:      &buf,
		InactivityTimeout: net.StreamInactivityTimeout,
	}
	err = client.PushImage(pushOpts, dockercommon.RegistryAuthConfig(imageName))
	if err != nil {
		log.Errorf("[docker] Failed to push image %q (%s): %s", opts.Name, err, buf.String())
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
