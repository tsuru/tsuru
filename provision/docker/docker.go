// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"errors"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"os/exec"
	"strings"
)

var fsystem fs.Fs

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}

// container represents an docker container with the given name.
type container struct {
	name       string
	instanceId string
}

// runCmd executes commands and log the given stdout and stderror.
func runCmd(cmd string, args ...string) (err error, output string) {
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()
	log.Printf("running the cmd: %s with the args: %s", cmd, args)
	output = string(out)
	return err, output
}

// ip returns the ip for the container.
func (c *container) ip() (string, error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return "", err
	}
	log.Printf("Getting ipaddress to instance %s", c.instanceId)
	err, instanceJson := runCmd("sudo", docker, "inspect", c.instanceId)
	if err != nil {
		log.Printf("error(%s) trying to inspect docker instance(%s) to get ipaddress", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(instanceJson), &result); err != nil {
		log.Printf("error(%s) parsing json from docker when trying to get ipaddress", err)
	}
	if ns, ok := result["NetworkSettings"]; !ok || ns == nil {
		msg := "Error when getting container information. NetworkSettings is missing."
		log.Printf(msg)
		return "", errors.New(msg)
	}
	networkSettings := result["NetworkSettings"].(map[string]interface{})
	instanceIp := networkSettings["IpAddress"].(string)
	if instanceIp == "" {
		log.Print("error: Can't get ipaddress...")
		return "", errors.New("Can't get ipaddress...")
	}
	log.Printf("Instance IpAddress: %s", instanceIp)
	return instanceIp, nil
}

// create creates a docker container with base template by default.
// TODO: this template already have a public key, we need to manage to install some way.
func (c *container) create() (instance_id string, err error) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return "", err
	}
	template, err := config.GetString("docker:image")
	if err != nil {
		return "", err
	}
	cmd, err := config.GetString("docker:cmd:bin")
	if err != nil {
		return "", err
	}
	args, err := config.GetList("docker:cmd:args")
	if err != nil {
		return "", err
	}
	args = append([]string{docker, "run", "-d", template, cmd}, args...)
	err, instance_id = runCmd("sudo", args...)
	instance_id = strings.Replace(instance_id, "\n", "", -1)
	log.Printf("docker instance_id=%s", instance_id)
	return instance_id, err
}

// start starts a docker container.
func (c *container) start() error {
	// it isn't necessary to start a docker container after docker run.
	return nil
}

// stop stops a docker container.
func (c *container) stop() error {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err
	}
	//TODO: better error handling
	log.Printf("trying to stop instance %s", c.instanceId)
	err, output := runCmd("sudo", docker, "stop", c.instanceId)
	log.Printf("docker stop=%s", output)
	return err
}

// destroy destory a docker container.
func (c *container) destroy() error {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err
	}
	//TODO: better error handling
	//TODO: Remove host's nginx route
	log.Printf("trying to destroy instance %s", c.instanceId)
	err, _ = runCmd("sudo", docker, "rm", c.instanceId)
	return err
}
