// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"os/exec"
	"strings"
    "encoding/json"
	"errors"
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
	name string
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
func (c *container) ip() (err error, ip string) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err, ""
	}
    log.Printf("Getting ipaddress to instance %s", c.instanceId)
    err, instance_json := runCmd("sudo", docker, "inspect", c.instanceId)
    if err != nil {
        log.Printf("error(%s) trying to inspect docker instance(%s) to get ipaddress",err)
    }
    var jsonBlob = []byte(instance_json)
    var result map[string]interface{}
    err2 := json.Unmarshal(jsonBlob, &result)
    NetworkSettings := result["NetworkSettings"].(map[string]interface{})
    if err2 != nil {
        log.Printf("error(%s) parsing jason from docker when trying to get ipaddress",err2)
    }
    instance_ip := NetworkSettings["IpAddress"].(string)
    if instance_ip != "" {
        log.Printf("Instance IpAddress: %s", instance_ip)
        return nil, instance_ip
    }
    log.Print("error: Can't get ipaddress...")
    return errors.New("Can't get ipaddress..."), ""
}

// create creates a docker container with base template by default.
// TODO: this template already have a public key, we need to manage to install some way.
func (c *container) create() (err error, instance_id string) {
	docker, err := config.GetString("docker:binary")
	if err != nil {
		return err, ""
	}
    err, instance_id = runCmd("sudo", docker, "run", "-d", "base-nginx-sshd-key", "/usr/sbin/sshd", "-D")
    instance_id = strings.Replace(instance_id, "\n", "", -1)
    log.Printf("docker instance_id=%s",instance_id)
	return err, instance_id
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
    log.Printf("docker stop=%s",output)
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
