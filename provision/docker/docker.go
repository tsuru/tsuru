// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/repository"
	"labix.org/v2/mgo/bson"
	"strings"
)

var fsystem fs.Fs

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}

// runCmd executes commands and log the given stdout and stderror.
func runCmd(cmd string, args ...string) (string, error) {
	out := bytes.Buffer{}
	err := executor().Execute(cmd, args, nil, &out, &out)
	log.Printf("running the cmd: %s with the args: %s", cmd, args)
	return out.String(), err
}

func runContainerCmd(app provision.App) ([]string, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return []string{}, err
	}
	repoNamespace, err := config.GetString("docker:repository-namespace")
	if err != nil {
		return []string{}, err
	}
	deployCmd, err := config.GetString("docker:deploy-cmd")
	if err != nil {
		return []string{}, err
	}
	appRepo := repository.GetReadOnlyUrl(app.GetName())
	runBin, err := config.GetString("docker:run-cmd:bin")
	if err != nil {
		return []string{}, err
	}
	runArgs, err := config.GetString("docker:run-cmd:args")
	if err != nil {
		return []string{}, err
	}
	port, err := config.GetString("docker:run-cmd:port")
	if err != nil {
		return []string{}, err
	}
	imageName := fmt.Sprintf("%s/%s", repoNamespace, app.GetPlatform()) // TODO (flaviamissi): should use same algorithm as image.repositoryName
	containerCmd := fmt.Sprintf("%s %s && %s %s", deployCmd, appRepo, runBin, runArgs)
	wholeCmd := []string{docker, "run", "-d", "-t", "-p", port, imageName, "/bin/bash", "-c", containerCmd}
	return wholeCmd, nil
}

// container represents an docker container with the given name.
type container struct {
	name string
	id   string
}

// newContainer creates a new container in docker and stores it on database.
//
// Receives the application to which the container is going to be generated
// and the function to assemble the command that the container will execute on
// startup.
// TODO (flaviamissi): make it atomic
func newContainer(app provision.App, f func(provision.App) ([]string, error)) (*container, error) {
	appName := app.GetName()
	c := &container{name: appName}
	id, err := c.create(app, f)
	if err != nil {
		log.Printf("Error creating container %s", appName)
		log.Printf("Error was: %s", err.Error())
		return c, err
	}
	c.id = id
	u := provision.Unit{
		Name:    id,
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
	}
	r, err := getRouter()
	if err != nil {
		return c, err
	}
	ip, err := c.ip()
	if err != nil {
		return c, err
	}
	u.Ip = ip
	if err := collection().Insert(u); err != nil {
		log.Print(err)
		return c, err
	}
	return c, r.AddRoute(app.GetName(), ip)
}

// ip returns the ip for the container.
func (c *container) ip() (string, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return "", err
	}
	log.Printf("Getting ipaddress to instance %s", c.id)
	instanceJson, err := runCmd(docker, "inspect", c.id)
	if err != nil {
		msg := "error(%s) trying to inspect docker instance(%s) to get ipaddress"
		log.Printf(msg, err)
		return "", errors.New(msg)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(instanceJson), &result); err != nil {
		msg := "error(%s) parsing json from docker when trying to get ipaddress"
		log.Printf(msg, err)
		return "", errors.New(msg)
	}
	if ns, ok := result["NetworkSettings"]; !ok || ns == nil {
		msg := "Error when getting container information. NetworkSettings is missing."
		log.Printf(msg)
		return "", errors.New(msg)
	}
	networkSettings := result["NetworkSettings"].(map[string]interface{})
	instanceIp := networkSettings["IpAddress"].(string)
	if instanceIp == "" {
		msg := "error: Can't get ipaddress..."
		log.Print(msg)
		return "", errors.New(msg)
	}
	log.Printf("Instance IpAddress: %s", instanceIp)
	return instanceIp, nil
}

// create creates a docker container with base template by default.
//
// It receives the application's platform in order to choose the correct
// docker image and the repository to pass to the script that will take
// care of the deploy, and a function to generate the correct command ran by
// docker, which might be to deploy a container or to run and expose a
// container for an application.
func (c *container) create(app provision.App, f func(provision.App) ([]string, error)) (string, error) {
	cmd, err := f(app)
	if err != nil {
		return "", err
	}
	id, err := runCmd(cmd[0], cmd[1:]...)
	id = strings.Replace(id, "\n", "", -1)
	log.Printf("docker id=%s", id)
	return id, err
}

// start starts a docker container.
func (c *container) start() error {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err
	}
	log.Printf("Stating container %s", c.id)
	out, err := runCmd(docker, "start", c.id)
	log.Printf("docker start output: %s", out)
	return err
}

// stop stops a docker container.
func (c *container) stop() error {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err
	}
	//TODO: better error handling
	log.Printf("Stopping container %s", c.id)
	output, err := runCmd(docker, "stop", c.id)
	log.Printf("docker stop output: %s", output)
	return err
}

// remove removes a docker container.
func (c *container) remove() error {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err
	}
	//TODO: better error handling
	//TODO: Remove host's nginx route
	log.Printf("trying to remove container %s", c.id)
	_, err = runCmd(docker, "rm", c.id)
	return err
}

// image represents a docker image.
type image struct {
	Name string
	Id   string
}

// repositoryName returns the image repository name for a given image.
//
// Repository is a docker concept, the image actually does not have a name,
// it has a repository, that is a composed name, e.g.: tsuru/base.
// Tsuru will always use a namespace, defined in tsuru.conf.
// Additionally, tsuru will use the application's name to do that composition.
func (img *image) repositoryName() string {
	repoNamespace, err := config.GetString("docker:repository-namespace")
	if err != nil {
		log.Printf("Tsuru is misconfigured. docker:repository-namespace config is missing.")
		return ""
	}
	return fmt.Sprintf("%s/%s", repoNamespace, img.Name)
}

// commit commits an image in docker
//
// This is another docker concept, in order to generate an image from a container
// one must commit it.
func (img *image) commit(cId string) (string, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		log.Printf("Tsuru is misconfigured. docker:binary config is missing.")
		return "", err
	}
	log.Printf("attempting to commit image from container %s", cId)
	rName := img.repositoryName()
	id, err := runCmd(docker, "commit", cId, rName)
	if err != nil {
		log.Printf("Could not commit docker image: %s", err.Error())
		return "", err
	}
	img.Id = strings.Replace(id, "\n", "", -1)
	if err := imagesCollection().Insert(&img); err != nil {
		log.Printf("Could not store image information %s", err.Error())
		return "", err
	}
	return img.Id, nil
}

// remove removes an image from docker registry
func (img *image) remove() error {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		log.Printf("Tsuru is misconfigured. docker:binary config is missing.")
		return err
	}
	log.Printf("attempting to remove image %s from docker", img.repositoryName())
	_, err = runCmd(docker, "rmi", img.Id)
	if err != nil {
		log.Printf("Could not remove image %s from docker: %s", img.Id, err.Error())
		return err
	}
	err = imagesCollection().Remove(bson.M{"name": img.Name})
	if err != nil {
		log.Printf("Could not remove image %s from mongo: %s", img.Id, err.Error())
		return err
	}
	return nil
}
