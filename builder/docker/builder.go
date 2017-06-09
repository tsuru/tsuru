package docker

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

var _ builder.Builder = &dockerBuilder{}

const (
	defaultArchiveName = "archive.tar.gz"
	defaultArchivePath = "/home/application"
)

type dockerBuilder struct{}

func init() {
	builder.Register("docker", &dockerBuilder{})
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
	user, err := config.GetString("docker:user")
	if err != nil {
		user, _ = config.GetString("docker:ssh:user")
	}
	imageName := image.GetBuildImage(app)
	options := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			AttachStdin:  true,
			User:         user,
			Image:        imageName,
		},
	}
	var cont *docker.Container
	cont, err = client.CreateContainer(options)
	if err != nil && err == docker.ErrNoSuchImage {
		w := evt
		fmt.Fprintln(w, "---- Pulling image to node ----")
		pullOpts := docker.PullImageOptions{
			Repository:        imageName,
			OutputStream:      w,
			InactivityTimeout: net.StreamInactivityTimeout,
		}
		err = client.PullImage(pullOpts, dockercommon.RegistryAuthConfig())
		if err != nil {
			return "", err
		}
		cont, err = client.CreateContainer(options)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	defer client.RemoveContainer(docker.RemoveContainerOptions{ID: cont.ID, Force: true})
	intermediateImageID, fileURI, err := dockercommon.UploadToContainer(client, cont.ID, tarFile)
	if err != nil {
		return "", err
	}
	defer client.RemoveImage(intermediateImageID)
	cmds := dockercommon.ArchiveDeployCmds(app, fileURI)
	imageID, err := b.buildPipeline(p, client, app, intermediateImageID, cmds, evt)
	if err != nil {
		return "", err
	}
	return imageID, nil
}

func imageBuild(client *docker.Client, app provision.App, imageID string, evt *event.Event) (string, error) {
	if !strings.Contains(imageID, ":") {
		imageID = fmt.Sprintf("%s:latest", imageID)
	}
	w := evt
	fmt.Fprintln(w, "---- Pulling image to tsuru ----")
	pullOpts := docker.PullImageOptions{
		Repository:        imageID,
		OutputStream:      w,
		InactivityTimeout: net.StreamInactivityTimeout,
	}
	err := client.PullImage(pullOpts, dockercommon.RegistryAuthConfig())
	if err != nil {
		return "", err
	}
	fmt.Fprintln(w, "---- Getting process from image ----")
	cmd := "(cat /home/application/current/Procfile || cat /app/user/Procfile || cat /Procfile || true) 2>/dev/null"
	var buf bytes.Buffer
	err = runCommandInContainer(client, imageID, cmd, app, &buf, nil)
	if err != nil {
		return "", err
	}
	newImage, err := dockercommon.PrepareImageForDeploy(dockercommon.PrepareImageArgs{
		Client:      client,
		App:         app,
		ProcfileRaw: buf.String(),
		ImageID:     imageID,
		Out:         w,
	})
	if err != nil {
		return "", err
	}
	app.SetUpdatePlatform(true)
	return newImage, nil
}

func runCommandInContainer(client *docker.Client, image string, command string, app provision.App, stdout, stderr io.Writer) error {
	createOptions := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			Image:        image,
			Entrypoint:   []string{"/bin/sh", "-c"},
			Cmd:          []string{command},
		},
	}
	cont, err := client.CreateContainer(createOptions)
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

func downloadFromContainer(client *docker.Client, app provision.App, filePath string) (io.ReadCloser, *docker.Container, error) {
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
	cont, err := client.CreateContainer(options)
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
