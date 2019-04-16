// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	provTypes "github.com/tsuru/tsuru/types/provision"
	yaml "gopkg.in/yaml.v2"
)

var _ builder.Builder = &dockerBuilder{}

const (
	defaultArchiveName = "archive.tar.gz"
	defaultArchivePath = "/home/application"
	procfileFileName   = "Procfile"
)

var (
	globalLimiter provision.ActionLimiter
	onceLimiter   sync.Once

	dirPaths = []string{
		filepath.Join(defaultArchivePath, "current"),
		"/app/user",
		"/",
	}

	tsuruYamlFiles = []string{"tsuru.yml", "tsuru.yaml", "app.yml", "app.yaml"}
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

func (b *dockerBuilder) Build(prov provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (string, error) {
	p, ok := prov.(provision.BuilderDeployDockerClient)
	if !ok {
		return "", errors.New("provisioner not supported: doesn't implement docker builder")
	}
	archiveFullPath := fmt.Sprintf("%s/%s", defaultArchivePath, defaultArchiveName)
	if opts.BuildFromFile {
		return "", errors.New("build image from Dockerfile is not yet supported")
	}
	client, err := p.GetClient(app)
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
		return imageBuild(client, app, opts, evt)
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

func imageBuild(client provision.BuilderDockerClient, app provision.App, opts *builder.BuildOpts, evt *event.Event) (string, error) {
	repo, tag := image.SplitImageName(opts.ImageID)
	imageID := fmt.Sprintf("%s:%s", repo, tag)
	fmt.Fprintln(evt, "---- Getting process from image ----")
	cmd := generateCatCommand([]string{procfileFileName}, dirPaths)
	var procfileBuf bytes.Buffer
	containerID, err := runCommandInContainer(client, evt, imageID, cmd, app, &procfileBuf, nil)
	defer removeContainer(client, containerID)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(evt, "---- Inspecting image %q ----\n", imageID)
	imageInspect, err := client.InspectImage(imageID)
	if err != nil {
		return "", err
	}
	if len(imageInspect.Config.ExposedPorts) > 1 {
		return "", errors.New("Too many ports. You should especify which one you want to.")
	}
	if _, ok := imageInspect.Config.Labels["is-tsuru"]; ok {
		opts.IsTsuruBuilderImage = true
	}
	procfile := image.GetProcessesFromProcfile(procfileBuf.String())
	if len(procfile) == 0 {
		fmt.Fprintln(evt, "  ---> Procfile not found, using entrypoint and cmd")
		procfile["web"] = append(imageInspect.Config.Entrypoint, imageInspect.Config.Cmd...)
	}
	for k, v := range procfile {
		fmt.Fprintf(evt, "  ---> Process %q found with commands: %q\n", k, v)
	}
	fmt.Fprintln(evt, "---- Getting tsuru.yaml from image ----")
	yaml, containerID, err := loadTsuruYaml(client, app, imageID, evt)
	defer removeContainer(client, containerID)
	if err != nil {
		return "", err
	}
	containerID, err = runBuildHooks(client, app, imageID, evt, yaml)
	defer removeContainer(client, containerID)
	if err != nil {
		return "", err
	}
	newImage, err := pushImageToRegistry(client, app, imageID, evt)
	if err != nil {
		return "", err
	}
	imageData := image.ImageMetadata{
		Name:         newImage,
		Processes:    procfile,
		CustomData:   tsuruYamlToCustomData(yaml),
		ExposedPorts: make([]string, len(imageInspect.Config.ExposedPorts)),
	}
	i := 0
	for k := range imageInspect.Config.ExposedPorts {
		imageData.ExposedPorts[i] = string(k)
		i++
	}
	err = imageData.Save()
	if err != nil {
		return "", err
	}
	return newImage, nil
}

func pushImageToRegistry(client provision.BuilderDockerClient, app provision.App, imageID string, evt *event.Event) (string, error) {
	newImage, err := image.AppNewImageName(app.GetName())
	if err != nil {
		return "", err
	}
	repo, tag := image.SplitImageName(newImage)
	err = client.TagImage(imageID, docker.TagImageOptions{Repo: repo, Tag: tag, Force: true})
	if err != nil {
		return "", err
	}
	registry, err := config.GetString("docker:registry")
	if err != nil {
		return "", err
	}
	fmt.Fprintf(evt, "---- Pushing image %q to tsuru ----\n", newImage)
	pushOpts := docker.PushImageOptions{
		Name:              repo,
		Tag:               tag,
		Registry:          registry,
		OutputStream:      &tsuruIo.DockerErrorCheckWriter{W: evt},
		InactivityTimeout: net.StreamInactivityTimeout,
		RawJSONStream:     true,
	}
	err = client.PushImage(pushOpts, dockercommon.RegistryAuthConfig(newImage))
	if err != nil {
		return "", err
	}
	return newImage, nil
}

func loadTsuruYaml(client provision.BuilderDockerClient, app provision.App, imageID string, evt *event.Event) (*provTypes.TsuruYamlData, string, error) {
	cmd := generateCatCommand(tsuruYamlFiles, dirPaths)
	var buf bytes.Buffer
	containerID, err := runCommandInContainer(client, evt, imageID, cmd, app, &buf, nil)
	if err != nil {
		return nil, containerID, err
	}
	var tsuruYamlData provTypes.TsuruYamlData
	err = yaml.Unmarshal(buf.Bytes(), &tsuruYamlData)
	if err != nil {
		return nil, containerID, err
	}
	return &tsuruYamlData, containerID, err
}

func tsuruYamlToCustomData(yaml *provTypes.TsuruYamlData) map[string]interface{} {
	if yaml == nil {
		return nil
	}

	return map[string]interface{}{
		"healthcheck": yaml.Healthcheck,
		"hooks":       yaml.Hooks,
	}
}

func runBuildHooks(client provision.BuilderDockerClient, app provision.App, imageID string, evt *event.Event, tsuruYamlData *provTypes.TsuruYamlData) (string, error) {
	if tsuruYamlData == nil || tsuruYamlData.Hooks == nil || len(tsuruYamlData.Hooks.Build) == 0 {
		return "", nil
	}
	cmd := strings.Join(tsuruYamlData.Hooks.Build, " && ")
	fmt.Fprintln(evt, "---- Running build hooks ----")
	fmt.Fprintf(evt, " ---> Running %q\n", cmd)
	containerID, err := runCommandInContainer(client, evt, imageID, cmd, app, evt, evt)
	if err != nil {
		return containerID, err
	}
	repo, tag := image.SplitImageName(imageID)
	opts := docker.CommitContainerOptions{
		Container:  containerID,
		Repository: repo,
		Tag:        tag,
	}
	newImage, err := client.CommitContainer(opts)
	if err != nil {
		return containerID, err
	}
	return newImage.ID, nil
}

func runCommandInContainer(client provision.BuilderDockerClient, evt *event.Event, imageID string, command string, app provision.App, stdout, stderr io.Writer) (string, error) {
	createOptions := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			Image:        imageID,
			Entrypoint:   []string{"/bin/sh", "-c"},
			Cmd:          []string{command},
		},
	}
	cont, _, err := client.PullAndCreateContainer(createOptions, evt)
	if err != nil {
		return "", err
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
		return cont.ID, err
	}
	<-attachOptions.Success
	close(attachOptions.Success)
	err = client.StartContainer(cont.ID, nil)
	if err != nil {
		return cont.ID, err
	}
	waiter.Wait()
	return cont.ID, nil
}

func removeContainer(client provision.BuilderDockerClient, containerID string) error {
	if containerID == "" {
		return nil
	}
	opts := docker.RemoveContainerOptions{
		ID:    containerID,
		Force: false,
	}
	return client.RemoveContainer(opts)
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
	client := net.Dial15Full300Client
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

func generateCatCommand(names []string, dirs []string) string {
	var cmds []string
	for _, name := range names {
		for _, dir := range dirs {
			path := filepath.Join(dir, name)
			cmds = append(cmds, fmt.Sprintf("cat %s", path))
		}
	}
	cmds = append(cmds, "true")
	return fmt.Sprintf("(%s) 2>/dev/null", strings.Join(cmds, " || "))
}
