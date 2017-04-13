package docker

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

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

type dockerBuilder struct {
}

func init() {
	builder.Register("docker", &dockerBuilder{})
}

func (b *dockerBuilder) Build(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts builder.BuildOpts) (string, error) {
	if opts.BuildFromFile {
		return "", errors.New("build image from Dockerfile is not yet supported")
	}
	client, err := p.GetDockerClient(app)
	if err != nil {
		return "", err
	}
	var tarFile io.ReadCloser
	if opts.ArchiveFile != nil && opts.ArchiveSize != 0 {
		tarFile = dockercommon.AddDeployTarFile(opts.ArchiveFile, opts.ArchiveSize, "archive.tar.gz")
	} else if opts.Rebuild {
		var rcont *docker.Container
		tarFile, rcont, err = downloadFromContainer(client, app, "/home/application/archive.tar.gz")
		if err != nil {
			return "", err
		}
		defer client.RemoveContainer(docker.RemoveContainerOptions{ID: rcont.ID, Force: true})
	} else if opts.ArchiveURL != "" {
		tarFile, err = downloadFromURL(opts.ArchiveURL)
		if err != nil {
			return "", err
		}
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
		err = client.PullImage(pullOpts, docker.AuthConfiguration{})
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
	cmds := dockercommon.ArchiveDeployCmds(app, fileURI)
	imageID, err := p.RunHookDeploy(client, app, intermediateImageID, cmds, evt)
	if err != nil {
		return "", err
	}
	return imageID, nil
}

func downloadFromContainer(client *docker.Client, app provision.App, filePath string) (io.ReadCloser, *docker.Container, error) {
	imageName, err := image.AppCurrentImageName(app.GetName())
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
	archiveFile, err := dockercommon.DownloadFromContainer(client, cont.ID, filePath)
	if err != nil {
		return nil, nil, errors.Errorf("App %s raw image not found", app.GetName())
	}
	return archiveFile, cont, nil
}

func downloadFromURL(url string) (io.ReadCloser, error) {
	var out bytes.Buffer
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, err = io.Copy(&out, resp.Body)
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(&out), nil
}
