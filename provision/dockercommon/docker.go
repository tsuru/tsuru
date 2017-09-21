// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/safe"
	"golang.org/x/net/context"
)

const (
	JsonFileLogDriver = "json-file"
)

type ClientWithTimeout struct {
	*docker.Client
}

func (c *ClientWithTimeout) SetTimeout(timeout time.Duration) {
	c.Client.HTTPClient.Timeout = timeout
}

type Client interface {
	PushImage(docker.PushImageOptions, docker.AuthConfiguration) error
	InspectImage(string) (*docker.Image, error)
	TagImage(string, docker.TagImageOptions) error
	CommitContainer(docker.CommitContainerOptions) (*docker.Image, error)
	DownloadFromContainer(string, docker.DownloadFromContainerOptions) error
}

func DownloadFromContainer(client Client, contID string, filePath string) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		downloadOpts := docker.DownloadFromContainerOptions{
			OutputStream: writer,
			Path:         filePath,
		}
		err := client.DownloadFromContainer(contID, downloadOpts)
		if err != nil {
			writer.CloseWithError(err)
		} else {
			writer.Close()
		}
	}()
	return reader, nil
}

type PrepareImageArgs struct {
	Client      Client
	App         provision.App
	ProcfileRaw string
	ImageID     string
	Out         io.Writer
}

func PrepareImageForDeploy(args PrepareImageArgs) (string, error) {
	fmt.Fprintf(args.Out, "---- Inspecting image %q ----\n", args.ImageID)
	procfile := image.GetProcessesFromProcfile(args.ProcfileRaw)
	imageInspect, err := args.Client.InspectImage(args.ImageID)
	if err != nil {
		return "", err
	}
	if len(procfile) == 0 {
		fmt.Fprintln(args.Out, "  ---> Procfile not found, using entrypoint and cmd")
		procfile["web"] = append(imageInspect.Config.Entrypoint, imageInspect.Config.Cmd...)
	}
	for k, v := range procfile {
		fmt.Fprintf(args.Out, "  ---> Process %q found with commands: %q\n", k, v)
	}
	newImage, err := image.AppNewImageName(args.App.GetName())
	if err != nil {
		return "", err
	}
	imageInfo := strings.Split(newImage, ":")
	repo, tag := strings.Join(imageInfo[:len(imageInfo)-1], ":"), imageInfo[len(imageInfo)-1]
	err = args.Client.TagImage(args.ImageID, docker.TagImageOptions{Repo: repo, Tag: tag, Force: true})
	if err != nil {
		return "", err
	}
	registry, err := config.GetString("docker:registry")
	if err != nil {
		return "", err
	}
	fmt.Fprintf(args.Out, "---- Pushing image %q to tsuru ----\n", newImage)
	pushOpts := docker.PushImageOptions{
		Name:              repo,
		Tag:               tag,
		Registry:          registry,
		OutputStream:      &tsuruIo.DockerErrorCheckWriter{W: args.Out},
		InactivityTimeout: net.StreamInactivityTimeout,
		RawJSONStream:     true,
	}
	err = args.Client.PushImage(pushOpts, RegistryAuthConfig())
	if err != nil {
		return "", err
	}
	imageData := image.ImageMetadata{
		Name:      newImage,
		Processes: procfile,
	}
	if len(imageInspect.Config.ExposedPorts) > 1 {
		return "", errors.New("Too many ports. You should especify which one you want to.")
	}
	for k := range imageInspect.Config.ExposedPorts {
		imageData.ExposedPort = string(k)
	}
	err = imageData.Save()
	if err != nil {
		return "", err
	}
	return newImage, nil
}

func WaitDocker(client *docker.Client) error {
	timeout, _ := config.GetInt("docker:api-timeout")
	if timeout == 0 {
		timeout = 600
	}
	timeoutChan := time.After(time.Duration(timeout) * time.Second)
	pong := make(chan error, 1)
	exit := make(chan struct{})
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := client.PingWithContext(ctx)
			cancel()
			if err == nil {
				pong <- nil
				return
			}
			if e, ok := err.(*docker.Error); ok && e.Status > 499 {
				pong <- err
				return
			}
			select {
			case <-exit:
				return
			case <-time.After(time.Second):
			}
		}
	}()
	select {
	case err := <-pong:
		return err
	case <-timeoutChan:
		close(exit)
		return errors.Errorf("Docker API at %q didn't respond after %d seconds", client.Endpoint(), timeout)
	}
}

func PushImage(client Client, name, tag string, authconfig docker.AuthConfiguration) error {
	if _, err := config.GetString("docker:registry"); err == nil {
		var buf safe.Buffer
		pushOpts := docker.PushImageOptions{
			Name:              name,
			Tag:               tag,
			OutputStream:      &buf,
			InactivityTimeout: net.StreamInactivityTimeout,
		}
		err = client.PushImage(pushOpts, authconfig)
		if err != nil {
			log.Errorf("[docker] Failed to push image %q (%s): %s", name, err, buf.String())
			return err
		}
	}
	return nil
}

func RegistryAuthConfig() docker.AuthConfiguration {
	var authConfig docker.AuthConfiguration
	authConfig.Email, _ = config.GetString("docker:registry-auth:email")
	authConfig.Username, _ = config.GetString("docker:registry-auth:username")
	authConfig.Password, _ = config.GetString("docker:registry-auth:password")
	authConfig.ServerAddress, _ = config.GetString("docker:registry")
	return authConfig
}
