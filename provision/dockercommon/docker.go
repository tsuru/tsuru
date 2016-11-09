// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"fmt"
	"io"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
)

type Client interface {
	PushImage(docker.PushImageOptions, docker.AuthConfiguration) error
	InspectImage(string) (*docker.Image, error)
	TagImage(string, docker.TagImageOptions) error
	UploadToContainer(string, docker.UploadToContainerOptions) error
	CommitContainer(docker.CommitContainerOptions) (*docker.Image, error)
}

func UploadToContainer(client Client, contID string, archiveFile io.Reader, fileSize int64) (string, string, error) {
	dirPath := "/home/application/"
	fileName := "archive.tar.gz"
	reader := addDeployTarFile(archiveFile, fileSize, fileName)
	uploadOpts := docker.UploadToContainerOptions{
		InputStream: reader,
		Path:        dirPath,
	}
	err := client.UploadToContainer(contID, uploadOpts)
	if err != nil {
		return "", "", err
	}
	image, err := client.CommitContainer(docker.CommitContainerOptions{
		Container: contID,
	})
	if err != nil {
		return "", "", err
	}
	return image.ID, fmt.Sprintf("file://%s%s", dirPath, fileName), nil
}

type PrepareImageArgs struct {
	Client      Client
	App         provision.App
	ProcfileRaw string
	ImageId     string
	AuthConfig  docker.AuthConfiguration
	Out         io.Writer
}

func PrepareImageForDeploy(args PrepareImageArgs) (string, error) {
	fmt.Fprintf(args.Out, "---- Inspecting image %q ----\n", args.ImageId)
	procfile := image.GetProcessesFromProcfile(args.ProcfileRaw)
	imageInspect, err := args.Client.InspectImage(args.ImageId)
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
	err = args.Client.TagImage(args.ImageId, docker.TagImageOptions{Repo: repo, Tag: tag, Force: true})
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
		OutputStream:      args.Out,
		InactivityTimeout: net.StreamInactivityTimeout,
		RawJSONStream:     true,
	}
	err = args.Client.PushImage(pushOpts, args.AuthConfig)
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
