// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bytes"
	"fmt"
	"github.com/globocom/tsuru/provision"
	"io"
	"launchpad.net/goyaml"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// JujuProvisioner is an implementation for the Provisioner interface. For more
// details on how a provisioner work, check the documentation of the provision
// package.
type JujuProvisioner struct{}

func (p *JujuProvisioner) Provision(app provision.App) error {
	var buf bytes.Buffer
	args := []string{
		"deploy", "--repository", "/home/charms",
		"local:" + app.GetFramework(), app.GetName(),
	}
	err := runCmd(&buf, &buf, args...)
	out := buf.String()
	if err != nil {
		app.Log("Failed to create machine: "+out, "tsuru")
		return &provision.Error{Reason: out, Err: err}
	}
	return nil
}

func (p *JujuProvisioner) Destroy(app provision.App) error {
	var buf bytes.Buffer
	err := runCmd(&buf, &buf, "destroy-service", app.GetName())
	out := buf.String()
	if err != nil {
		app.Log("Failed to destroy machine: "+out, "tsuru")
		return &provision.Error{Reason: out, Err: err}
	}
	for _, u := range app.ProvisionUnits() {
		buf.Reset()
		err = runCmd(&buf, &buf, "terminate-machine", strconv.Itoa(u.GetMachine()))
		out = buf.String()
		if err != nil {
			app.Log("Failed to destroy machine: "+out, "tsuru")
			return &provision.Error{Reason: out, Err: err}
		}
	}
	return nil
}

func (p *JujuProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	arguments := []string{"ssh", "-o", "StrictHostKeyChecking no", "-q"}
	for _, unit := range app.ProvisionUnits() {
		var cmdargs []string
		cmdargs = append(cmdargs, arguments...)
		cmdargs = append(cmdargs, strconv.Itoa(unit.GetMachine()), cmd)
		cmdargs = append(cmdargs, args...)
		err := runCmd(stdout, stderr, cmdargs...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *JujuProvisioner) CollectStatus() ([]provision.Unit, error) {
	output, err := execWithTimeout(30e9, "juju", "status")
	if err != nil {
		return nil, &provision.Error{Reason: string(output), Err: err}
	}
	var out jujuOutput
	err = goyaml.Unmarshal(output, &out)
	if err != nil {
		return nil, &provision.Error{Reason: `"juju status" returned invalid data`, Err: err}
	}
	var units []provision.Unit
	for name, service := range out.Services {
		for _, u := range service.Units {
			machine := out.Machines[u.Machine]
			unit := provision.Unit{
				Name:    machine.InstanceId,
				AppName: name,
				Machine: u.Machine,
				Ip:      machine.IpAddress,
			}
			typeRegexp := regexp.MustCompile(`^(local:)?(\w+)/(\w+)-\d+$`)
			matchs := typeRegexp.FindStringSubmatch(service.Charm)
			if len(matchs) > 3 {
				unit.Type = matchs[3]
			}
			unit.Status = unitStatus(machine.InstanceState, u.AgentState, machine.AgentState)
			units = append(units, unit)
		}
	}
	return units, nil
}

type unit struct {
	AgentState string `yaml:"agent-state"`
	Machine    int
}

type service struct {
	Units map[string]unit
	Charm string
}

type machine struct {
	AgentState    string `yaml:"agent-state"`
	IpAddress     string `yaml:"dns-name"`
	InstanceId    string `yaml:"instance-id"`
	InstanceState string `yaml:"instance-state"`
}

type jujuOutput struct {
	Services map[string]service
	Machines map[int]machine
}

func init() {
	provision.Register("juju", &JujuProvisioner{})
}

func runCmd(stdout, stderr io.Writer, cmd ...string) error {
	stdout = &Writer{stdout}
	stderr = &Writer{stderr}
	command := exec.Command("juju", cmd...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func execWithTimeout(timeout time.Duration, cmd string, args ...string) (output []byte, err error) {
	var buf bytes.Buffer
	ch := make(chan []byte, 1)
	errCh := make(chan error, 1)
	command := exec.Command(cmd, args...)
	command.Stdout = &buf
	command.Stderr = &buf
	if err = command.Start(); err != nil {
		return nil, err
	}
	go func() {
		if err := command.Wait(); err == nil {
			ch <- buf.Bytes()
		} else {
			errCh <- err
			ch <- buf.Bytes()
		}
	}()
	select {
	case output = <-ch:
		select {
		case err = <-errCh:
		case <-time.After(1e9):
		}
	case err = <-errCh:
		output = <-ch
	case <-time.After(timeout):
		argsStr := strings.Join(args, " ")
		err = fmt.Errorf("%q ran for more than %s.", cmd+" "+argsStr, timeout)
		command.Process.Kill()
	}
	return output, err
}

func unitStatus(instanceState, agentState, machineAgentState string) string {
	if instanceState == "error" || agentState == "install-error" || machineAgentState == "start-error" {
		return provision.StatusError
	}
	if machineAgentState == "pending" || machineAgentState == "not-started" || machineAgentState == "" {
		return provision.StatusCreating
	}
	if instanceState == "pending" || instanceState == "" {
		return provision.StatusCreating
	}
	if agentState == "down" {
		return provision.StatusDown
	}
	if machineAgentState == "running" && agentState == "not-started" {
		return provision.StatusCreating
	}
	if machineAgentState == "running" && instanceState == "running" && agentState == "pending" {
		return provision.StatusInstalling
	}
	if machineAgentState == "running" && agentState == "started" && instanceState == "running" {
		return provision.StatusStarted
	}
	return provision.StatusPending
}
