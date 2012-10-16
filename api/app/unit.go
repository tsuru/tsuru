// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/log"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type Unit struct {
	Type              string
	Name              string
	Machine           int
	Ip                string
	AgentState        string `yaml:"agent-state"`
	MachineAgentState string
	InstanceState     string
	InstanceId        string
	app               *App
}

func (u *Unit) destroy() ([]byte, error) {
	if u.Machine < 1 {
		return nil, errors.New("No machine associated.")
	}
	cmd := exec.Command("juju", "destroy-service", u.app.Name)
	log.Printf("destroying %s with name %s", u.Type, u.Name)
	out, err := cmd.CombinedOutput()
	log.Printf(string(out))
	if err != nil {
		return out, err
	}
	cmd = exec.Command("juju", "terminate-machine", strconv.Itoa(u.Machine))
	return cmd.CombinedOutput()
}

func (u *Unit) executeHook(hook string, stdout, stderr io.Writer) ([]byte, error) {
	cmd := fmt.Sprintf("/var/lib/tsuru/hooks/%s", hook)
	output, err := u.Command(stdout, stderr, cmd)
	log.Print(string(output))
	return output, err
}

func (u *Unit) Command(stdout, stderr io.Writer, cmds ...string) ([]byte, error) {
	if state := u.State(); state != "started" {
		return nil, fmt.Errorf("Unit must be started to run commands, but it is %s.", state)
	}
	c := exec.Command("juju", "ssh", "-o", "StrictHostKeyChecking no", "-q", strconv.Itoa(u.Machine))
	c.Args = append(c.Args, cmds...)
	log.Printf("executing %s on %s", strings.Join(cmds, " "), u.app.Name)
	var b bytes.Buffer
	if stdout == nil {
		stdout = &b
	}
	if stderr == nil {
		stderr = &b
	}
	c.Stdout = stdout
	c.Stderr = stderr
	err := c.Run()
	return b.Bytes(), err
}

func (u *Unit) GetName() string {
	return u.app.Name
}

func (u *Unit) GetIp() string {
	return u.Ip
}

func (u *Unit) State() string {
	if u.InstanceState == "error" || u.AgentState == "install-error" || u.MachineAgentState == "start-error" {
		return "error"
	}
	if u.MachineAgentState == "pending" || u.MachineAgentState == "not-started" || u.MachineAgentState == "" {
		return "creating"
	}
	if u.InstanceState == "pending" || u.InstanceState == "" {
		return "creating"
	}
	if u.AgentState == "down" {
		return "down"
	}
	if u.MachineAgentState == "running" && u.AgentState == "not-started" {
		return "creating"
	}
	if u.MachineAgentState == "running" && u.InstanceState == "running" && u.AgentState == "pending" {
		return "installing"
	}
	if u.MachineAgentState == "running" && u.AgentState == "started" && u.InstanceState == "running" {
		return "started"
	}
	return "pending"
}
