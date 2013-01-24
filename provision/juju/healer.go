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

// NeedsHeal always returns true.
func (h *BootstrapProvisionHealer) NeedsHeal() bool {
	return true
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

func upStartCmd(cmd, daemon, machine string) error {
	args := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		machine,
		"sudo",
		cmd,
		daemon,
	}
	log.Printf(strings.Join(args, " "))
	c := exec.Command("ssh", args...)
	return c.Run()
}

// Heal executes the action for heal the bootstrap machine agent.
func (h *BootstrapMachineHealer) Heal() error {
	if h.NeedsHeal() {
		bootstrapMachine := getBootstrapMachine()
		log.Printf("Healing bootstrap juju-machine-agent")
		upStartCmd("stop", "juju-machine-agent", bootstrapMachine.IpAddress)
		return upStartCmd("start", "juju-machine-agent", bootstrapMachine.IpAddress)
	}
	log.Printf("Bootstrap juju-machine-agent needs no cure, skipping...")
	return nil
}
