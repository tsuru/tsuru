// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/url"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/safe"
)

var _ builder.Builder = &dockerBuilder{}

func (b *dockerBuilder) PlatformAdd(opts builder.PlatformOptions) error {
	return b.buildPlatform(opts.Name, opts.Args, opts.Output, opts.Input)
}

func (b *dockerBuilder) PlatformUpdate(opts builder.PlatformOptions) error {
	return b.buildPlatform(opts.Name, opts.Args, opts.Output, opts.Input)
}

func (b *dockerBuilder) buildPlatform(name string, args map[string]string, w io.Writer, r io.Reader) error {
	var inputStream io.Reader
	var dockerfileURL string
	if r != nil {
		data, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		writer := tar.NewWriter(&buf)
		writer.WriteHeader(&tar.Header{
			Name: "Dockerfile",
			Mode: 0644,
			Size: int64(len(data)),
		})
		writer.Write(data)
		writer.Close()
		inputStream = &buf
	} else {
		dockerfileURL = args["dockerfile"]
		if dockerfileURL == "" {
			return errors.New("Dockerfile is required")
		}
		if _, err := url.ParseRequestURI(dockerfileURL); err != nil {
			return errors.New("Dockerfile parameter must be a URL")
		}
	}
	imageName := image.PlatformImageName(name)
	client, err := getDockerClient()
	if err != nil {
		return err
	}
	client.HTTPClient.Timeout = 0
	buildOptions := docker.BuildImageOptions{
		Name:              imageName,
		Pull:              true,
		NoCache:           true,
		RmTmpContainer:    true,
		Remote:            dockerfileURL,
		InputStream:       inputStream,
		OutputStream:      w,
		InactivityTimeout: net.StreamInactivityTimeout,
		RawJSONStream:     true,
	}
	err = client.BuildImage(buildOptions)
	if err != nil {
		return err
	}
	parts := strings.Split(imageName, ":")
	var tag string
	if len(parts) > 2 {
		imageName = strings.Join(parts[:len(parts)-1], ":")
		tag = parts[len(parts)-1]
	} else if len(parts) > 1 {
		imageName = parts[0]
		tag = parts[1]
	} else {
		imageName = parts[0]
		tag = "latest"
	}
	var buf safe.Buffer
	pushOpts := docker.PushImageOptions{
		Name:              imageName,
		Tag:               tag,
		OutputStream:      &buf,
		InactivityTimeout: net.StreamInactivityTimeout,
		RawJSONStream:     true,
	}
	err = client.PushImage(pushOpts, dockercommon.RegistryAuthConfig())
	if err != nil {
		log.Errorf("[docker] Failed to push image %q (%s): %s", name, err, buf.String())
		return err
	}
	return nil
}

func getDockerClient() (*docker.Client, error) {
	provisioners, err := provision.Registry()
	if err != nil {
		return nil, err
	}
	var client *docker.Client
	multiErr := tsuruErrors.NewMultiError()
	for _, p := range provisioners {
		if provisioner, ok := p.(provision.BuilderDeploy); ok {
			client, err = provisioner.GetDockerClient(nil)
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
