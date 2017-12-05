// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	yaml "gopkg.in/yaml.v2"
)

var _ builder.Builder = &dockerBuilder{}

const (
	defaultArchiveName = "archive.tar.gz"
	defaultArchivePath = "/home/application"
)

var (
	globalLimiter provision.ActionLimiter
	onceLimiter   sync.Once
)

type dockerBuilder struct{}

func init() {
	builder.Register("docker", &dockerBuilder{})
}

func limiter() provision.ActionLimiter {
	onceLimiter.Do(func() {
		limitMode, _ := config.GetString("docker:limit:mode")
		if limitMode == "global" {
			globalLimiter = &provision.MongodbLimiter{}
		} else {
			globalLimiter = &provision.LocalLimiter{}
		}
		actionLimit, _ := config.GetUint("docker:limit:actions-per-host")
		if actionLimit > 0 {
			globalLimiter.Initialize(actionLimit)
		}
	})
	return globalLimiter
}

func (b *dockerBuilder) Build(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts builder.BuildOpts) (string, error) {
	archiveFullPath := fmt.Sprintf("%s/%s", defaultArchivePath, defaultArchiveName)
	if opts.BuildFromFile {
		return "", errors.New("build image from Dockerfile is not yet supported")
	}
	client, err := p.GetDockerClient(app)
	if err != nil {
		return "", err
	}
	var tarFile io.ReadCloser
	if opts.ArchiveFile != nil && opts.ArchiveSize != 0 {
		tarFile = dockercommon.AddDeployTarFile(opts.ArchiveFile, opts.ArchiveSize, defaultArchiveName)
	} else if opts.Rebuild {
		var rcont *docker.Container
		tarFile, rcont, err = downloadFromContainer(client, app, archiveFullPath)
		if err != nil {
			return "", err
		}
		defer client.RemoveContainer(docker.RemoveContainerOptions{ID: rcont.ID, Force: true})
	} else if opts.ArchiveURL != "" {
		tarFile, err = downloadFromURL(opts.ArchiveURL)
		if err != nil {
			return "", err
		}
	} else if opts.ImageID != "" {
		return imageBuild(client, app, opts.ImageID, evt)
	} else {
		return "", errors.New("no valid files found")
	}
	defer tarFile.Close()
	imageID, err := b.buildPipeline(p, client, app, tarFile, evt, opts.Tag)
	if err != nil {
		return "", err
	}
	return imageID, nil
}

func imageBuild(client provision.BuilderDockerClient, app provision.App, imageID string, evt *event.Event) (string, error) {
	if !strings.Contains(imageID, ":") {
		imageID = fmt.Sprintf("%s:latest", imageID)
	}

	fmt.Fprintln(evt, "---- Getting process from image ----")
	cmd := "(cat /home/application/current/Procfile || cat /app/user/Procfile || cat /Procfile || true) 2>/dev/null"
	var procfileBuf bytes.Buffer
	err := runCommandInContainer(client, evt, imageID, cmd, app, &procfileBuf, nil)
	if err != nil {
		return "", err
	}

	fmt.Fprintln(evt, "---- Getting tsuru.yaml from image ----")
	customData, err := loadTsuruYaml(client, app, imageID, evt)
	if err != nil {
		return "", err
	}

	newImage, err := dockercommon.PrepareImageForDeploy(dockercommon.PrepareImageArgs{
		Client:      client,
		App:         app,
		ProcfileRaw: procfileBuf.String(),
		ImageID:     imageID,
		Out:         evt,
		CustomData:  customData,
	})
	if err != nil {
		return "", err
	}
	return newImage, nil
}

func loadTsuruYaml(client provision.BuilderDockerClient, app provision.App, imageID string, evt *event.Event) (map[string]interface{}, error) {
	path := defaultArchivePath + "/current"
	cmd := fmt.Sprintf("(cat %s/tsuru.yml || cat %s/tsuru.yaml || cat %s/app.yml || cat %s/app.yaml || true) 2>/dev/null", path, path, path, path)
	var buf bytes.Buffer
	err := runCommandInContainer(client, evt, imageID, cmd, app, &buf, nil)
	if err != nil {
		return nil, err
	}
	var tsuruYamlData provision.TsuruYamlData
	err = yaml.Unmarshal(buf.Bytes(), &tsuruYamlData)
	if err != nil {
		return nil, err
	}
	customData := map[string]interface{}{
		"healthcheck": tsuruYamlData.Healthcheck,
		"hooks": map[string]interface{}{
			"build": tsuruYamlData.Hooks.Build,
			"restart": map[string]interface{}{
				"before": tsuruYamlData.Hooks.Restart.Before,
				"after":  tsuruYamlData.Hooks.Restart.After,
			},
		},
	}
	return customData, err
}

func runCommandInContainer(client provision.BuilderDockerClient, evt *event.Event, image string, command string, app provision.App, stdout, stderr io.Writer) error {
	createOptions := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			Image:        image,
			Entrypoint:   []string{"/bin/sh", "-c"},
			Cmd:          []string{command},
		},
	}
	cont, _, err := client.PullAndCreateContainer(createOptions, evt)
	if err != nil {
		return err
	}
	attachOptions := docker.AttachToContainerOptions{
		Container:    cont.ID,
		OutputStream: stdout,
		ErrorStream:  stderr,
		Stream:       true,
		Stdout:       true,
		Stderr:       true,
		Success:      make(chan struct{}),
	}
	waiter, err := client.AttachToContainerNonBlocking(attachOptions)
	if err != nil {
		return err
	}
	<-attachOptions.Success
	close(attachOptions.Success)
	err = client.StartContainer(cont.ID, nil)
	if err != nil {
		return err
	}
	waiter.Wait()
	return nil
}

func downloadFromContainer(client provision.BuilderDockerClient, app provision.App, filePath string) (io.ReadCloser, *docker.Container, error) {
	imageName, err := image.AppCurrentBuilderImageName(app.GetName())
	if err != nil {
		return nil, nil, errors.Errorf("App %s image not found", app.GetName())
	}
	options := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			Image:        imageName,
		},
	}
	cont, _, err := client.PullAndCreateContainer(options, nil)
	if err != nil {
		return nil, nil, err
	}
	archiveFile, err := dockercommon.DownloadFromContainer(client, cont.ID, filePath)
	if err != nil {
		return nil, nil, errors.Errorf("App %s raw image not found", app.GetName())
	}
	return archiveFile, cont, nil
}

func downloadFromURL(url string) (io.ReadCloser, error) {
	var out bytes.Buffer
	client := net.Dial5Full300Client
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	s, err := io.Copy(&out, resp.Body)
	if err != nil {
		return nil, err
	}
	if s == 0 {
		return nil, errors.New("archive file is empty")
	}
	return ioutil.NopCloser(&out), nil
}
