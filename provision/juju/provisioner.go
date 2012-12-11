// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/tsuru/provision"
	"io"
	"os/exec"
	"strconv"
)

func init() {
	provision.Register("juju", &JujuProvisioner{})
}

// JujuProvisioner is an implementation for Provisioner interface. For more
// details on how a provisioner work, check the documentation of the provision
// package.
type JujuProvisioner struct{}

func (p *JujuProvisioner) Provision(app provision.App) *provision.Error {
	args := []string{
		"deploy", "--repository", "/home/charms",
		"local:" + app.GetFramework(), app.GetName(),
	}
	cmd := exec.Command("juju", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		app.Log("Failed to create machine: "+string(out), "tsuru")
		return &provision.Error{Reason: string(out), Err: err}
	}
	return nil
}

func (p *JujuProvisioner) Destroy(app provision.App) *provision.Error {
	cmd := exec.Command("juju", "destroy-service", app.GetName())
	out, err := cmd.CombinedOutput()
	if err != nil {
		app.Log("Failed to destroy machine: "+string(out), "tsuru")
		return &provision.Error{Reason: string(out), Err: err}
	}
	for _, u := range app.GetUnits() {
		cmd = exec.Command("juju", "terminate-machine", strconv.Itoa(u.GetMachine()))
		out, err = cmd.CombinedOutput()
		if err != nil {
			app.Log("Failed to destroy machine: "+string(out), "tsuru")
			return &provision.Error{Reason: string(out), Err: err}
		}
	}
	return nil
}

func (p *JujuProvisioner) ExecuteCommand(w io.Writer, app provision.App, cmd string, args ...string) error {
	arguments := []string{"ssh", "-o", "StrictHostKeyChecking no", "-q"}
	for _, unit := range app.GetUnits() {
		var cmdargs []string
		cmdargs = append(cmdargs, arguments...)
		cmdargs = append(cmdargs, strconv.Itoa(unit.GetMachine()), cmd)
		cmdargs = append(cmdargs, args...)
		cmd := exec.Command("juju", cmdargs...)
		cmd.Stdout = w
		cmd.Stderr = w
		err := cmd.Run()
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *JujuProvisioner) CollectStatus() []provision.Unit {
	return nil
}
