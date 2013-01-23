// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/tsuru/heal"
	"github.com/globocom/tsuru/log"
	"os/exec"
	"strings"
)

func init() {
	heal.Register("bootstrap", &BootstrapMachineHealer{})
	heal.Register("bootstrap-provision", &BootstrapProvisionHealer{})
}

// BootstrapProvisionHealer is an import for the Healer interface. For more
// details on how a healer work, check the documentation of the heal package.
type BootstrapProvisionHealer struct{}

func (h *BootstrapProvisionHealer) NeedsHeal() bool {
	return false
}

func (h *BootstrapProvisionHealer) Heal() error {
	return nil
}

// BootstrapMachineHealer is an implementation for the Healer interface. For more
// details on how a healer work, check the documentation of the heal package.
type BootstrapMachineHealer struct{}

// getBootstrapMachine returns the bootstrap machine.
func getBootstrapMachine() machine {
	p := JujuProvisioner{}
	output, _ := p.getOutput()
	// for juju bootstrap machine always is the machine 0.
	return output.Machines[0]
}

// NeedsHeal returns true if the AgentState of bootstrap machine is "not-started".
func (h *BootstrapMachineHealer) NeedsHeal() bool {
	bootstrapMachine := getBootstrapMachine()
	return bootstrapMachine.AgentState == "not-started"
}

// Heal executes the action for heal the bootstrap machine agent.
func (h *BootstrapMachineHealer) Heal() error {
	if h.NeedsHeal() {
		bootstrapMachine := getBootstrapMachine()
		args := []string{
			"-o",
			"StrictHostKeyChecking no",
			"-q",
			"-l",
			"ubuntu",
			bootstrapMachine.IpAddress,
			"sudo",
			"start",
			"juju-machine-agent",
		}
		cmd := exec.Command("ssh", args...)
		log.Printf("Healing bootstrap juju-machine-agent")
		log.Printf(strings.Join(args, " "))
		return cmd.Run()
	}
	log.Printf("Bootstrap juju-machine-agent needs no cure, skipping...")
	return nil
}
