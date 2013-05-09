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
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"os"
	"os/user"
	"path"
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

func getSSHCommands() ([]string, error) {
	addKeyCommand, err := config.GetString("docker:ssh:add-key-cmd")
	if err != nil {
		return nil, err
	}
	keyFile, err := config.GetString("docker:ssh:public-key")
	if err != nil {
		if u, err := user.Current(); err == nil {
			keyFile = path.Join(u.HomeDir, ".ssh", "id_rsa.pub")
		} else {
			keyFile = os.ExpandEnv("${HOME}/.ssh/id_rsa.pub")
		}
	}
	f, err := filesystem().Open(keyFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	keyContent, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	sshdPath, err := config.GetString("docker:ssh:sshd-path")
	if err != nil {
		sshdPath = "/usr/sbin/sshd"
	}
	return []string{
		fmt.Sprintf("%s %s", addKeyCommand, bytes.TrimSpace(keyContent)),
		sshdPath,
	}, nil
}

func runContainerCmd(app provision.App) ([]string, string, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return nil, "", err
	}
	repoNamespace, err := config.GetString("docker:repository-namespace")
	if err != nil {
		return nil, "", err
	}
	deployCmd, err := config.GetString("docker:deploy-cmd")
	if err != nil {
		return nil, "", err
	}
	appRepo := repository.GetReadOnlyUrl(app.GetName())
	runBin, err := config.GetString("docker:run-cmd:bin")
	if err != nil {
		return nil, "", err
	}
	runArgs, err := config.GetString("docker:run-cmd:args")
	if err != nil {
		return nil, "", err
	}
	port, err := config.GetString("docker:run-cmd:port")
	if err != nil {
		return nil, "", err
	}
	commands, err := getSSHCommands()
	if err != nil {
		return nil, "", err
	}
	commands = append(commands, fmt.Sprintf("%s %s", deployCmd, appRepo), fmt.Sprintf("%s %s", runBin, runArgs))
	imageName := fmt.Sprintf("%s/%s", repoNamespace, app.GetPlatform()) // TODO (flaviamissi): should use same algorithm as image.repositoryName
	containerCmd := strings.Join(commands, " && ")
	wholeCmd := []string{docker, "run", "-d", "-t", "-p", port, imageName, "/bin/bash", "-c", containerCmd}
	return wholeCmd, port, nil
}

type container struct {
	Id      string `bson:"_id"`
	AppName string
	Type    string
	Ip      string
	Port    string
}

// newContainer creates a new container in Docker and stores it in the database.
//
// TODO (flaviamissi): make it atomic
func newContainer(app provision.App) (*container, error) {
	appName := app.GetName()
	c := container{
		AppName: appName,
		Type:    app.GetPlatform(),
	}
	err := c.create(app)
	if err != nil {
		log.Printf("Error creating container %s", appName)
		log.Printf("Error was: %s", err.Error())
		return nil, err
	}
	return &c, nil
}

func (c *container) inspect() (map[string]interface{}, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return nil, err
	}
	out, err := runCmd(docker, "inspect", c.Id)
	if err != nil {
		return nil, err
	}
	var r map[string]interface{}
	err = json.Unmarshal([]byte(out), &r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// hostPort returns the host port mapped for the container.
func (c *container) hostPort() (string, error) {
	if c.Port == "" {
		return "", errors.New("Container does not contain any mapped port")
	}
	data, err := c.inspect()
	if err != nil {
		return "", err
	}
	mappedPorts := data["NetworkSettings"].(map[string]interface{})["PortMapping"].(map[string]interface{})
	if port, ok := mappedPorts[c.Port]; ok {
		return port.(string), nil
	}
	return "", fmt.Errorf("Container port %s is not mapped to any host port", c.Port)
}

// ip returns the ip for the container.
func (c *container) ip() (string, error) {
	result, err := c.inspect()
	if err != nil {
		msg := fmt.Sprintf("error(%s) parsing json from docker when trying to get ipaddress", err)
		log.Print(msg)
		return "", errors.New(msg)
	}
	if ns, ok := result["NetworkSettings"]; !ok || ns == nil {
		msg := "Error when getting container information. NetworkSettings is missing."
		log.Print(msg)
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
func (c *container) create(app provision.App) error {
	hostAddr, err := config.Get("docker:host-address")
	if err != nil {
		return err
	}
	cmd, port, err := runContainerCmd(app)
	if err != nil {
		return err
	}
	id, err := runCmd(cmd[0], cmd[1:]...)
	if err != nil {
		return err
	}
	id = strings.Replace(id, "\n", "", -1)
	log.Printf("docker id=%s", id)
	c.Id = strings.TrimSpace(id)
	c.Port = port
	ip, err := c.ip()
	if err != nil {
		return err
	}
	c.Ip = ip
	coll := collection()
	defer coll.Database.Session.Close()
	if err := coll.Insert(c); err != nil {
		log.Print(err)
		return err
	}
	r, err := getRouter()
	if err != nil {
		return err
	}
	hostPort, err := c.hostPort()
	if err != nil {
		hostPort = c.Port
	}
	return r.AddRoute(app.GetName(), fmt.Sprintf("%s:%s", hostAddr, hostPort))
}

// start starts a docker container.
func (c *container) start() error {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err
	}
	log.Printf("Stating container %s", c.Id)
	out, err := runCmd(docker, "start", c.Id)
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
	log.Printf("Stopping container %s", c.Id)
	output, err := runCmd(docker, "stop", c.Id)
	log.Printf("docker stop output: %s", output)
	return err
}

// remove removes a docker container.
func (c *container) remove() error {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err
	}
	log.Printf("Removing container %s from docker", c.Id)
	out, err := runCmd(docker, "rm", c.Id)
	if err != nil {
		log.Printf("Failed to remove container from docker: %s", err.Error())
		log.Printf("Command output: %s", out)
		return err
	}
	log.Printf("Removing container %s from database", c.Id)
	coll := collection()
	defer coll.Database.Session.Close()
	if err := coll.RemoveId(c.Id); err != nil {
		log.Printf("Failed to remove container from database: %s", err.Error())
		return err
	}
	r, err := getRouter()
	if err != nil {
		log.Printf("Failed to obtain router: %s", err.Error())
		return err
	}
	if err := r.RemoveRoute(c.AppName); err != nil {
		log.Printf("Failed to remove route: %s", err.Error())
		return err
	}
	return nil
}

func (c *container) ssh(cmd string, args ...string) (string, error) {
	user, err := config.GetString("docker:ssh:user")
	if err != nil {
		return "", err
	}
	sshArgs := []string{c.Ip, "-l", user, "-o", "StrictHostKeyChecking no"}
	if keyFile, err := config.GetString("docker:ssh:private-key"); err == nil {
		sshArgs = append(sshArgs, "-i", keyFile)
	}
	sshArgs = append(sshArgs, "--", cmd)
	sshArgs = append(sshArgs, args...)
	return runCmd("ssh", sshArgs...)
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

func getContainer(id string) (*container, error) {
	var c container
	coll := collection()
	defer coll.Database.Session.Close()
	err := coll.Find(bson.M{"_id": id}).One(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func getContainers(appName string) ([]container, error) {
	var containers []container
	err := collection().Find(bson.M{"appname": appName}).All(&containers)
	return containers, err
}
