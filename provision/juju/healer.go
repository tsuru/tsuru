// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bufio"
	"fmt"
	"github.com/globocom/tsuru/heal"
	"github.com/globocom/tsuru/log"
	"net"
	"os/exec"
	"strings"
)

func init() {
	heal.Register("bootstrap", &BootstrapMachineHealer{})
	heal.Register("bootstrap-provision", &BootstrapProvisionHealer{})
	heal.Register("instance-machine", &InstanceMachineHealer{})
	heal.Register("instance-unit", &InstanceUnitHealer{})
	heal.Register("zookeeper", &ZookeeperHealer{})
}

// InstanceUnitHealer is an implementation for the Healer interface. For more
// detail on how a healer work, check the documentation of the heal package.
type InstanceUnitHealer struct{}

func (h *InstanceUnitHealer) Heal() error {
	return nil
}

// InstanceMachineHealer is an implementation for the Healer interface. For more
// detail on how a healer work, check the documentation of the heal package.
type InstanceMachineHealer struct{}

// Heal iterates through all juju machines verifying if
// a juju-machine-agent is down and heal these machines.
func (h *InstanceMachineHealer) Heal() error {
	p := JujuProvisioner{}
	output, _ := p.getOutput()
	for _, machine := range output.Machines {
		if machine.AgentState == "down" {
			log.Printf("Healing juju-machine-agent in machine %s", machine.InstanceId)
			upStartCmd("stop", "juju-machine-agent", machine.IpAddress)
			upStartCmd("start", "juju-machine-agent", machine.IpAddress)
		} else {
			log.Printf("juju-machine-agent for machine %s needs no cure, skipping...", machine.InstanceId)
		}
	}
	return nil
}

// ZookeeperHealer is an implementation for the Healer interface. For more
// detail on how a healer work, check the documentation of the heal package.
type ZookeeperHealer struct{}

// needsHeal verifies if zookeeper is ok using 'ruok' command.
func (h *ZookeeperHealer) needsHeal() bool {
	bootstrapMachine := getBootstrapMachine()
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:2181", bootstrapMachine.IpAddress))
	if err != nil {
		return true
	}
	defer conn.Close()
	fmt.Fprintf(conn, "ruok\r\n\r\n")
	status, _ := bufio.NewReader(conn).ReadString('\n')
	return !strings.Contains(status, "imok")
}

// Heal restarts the zookeeper using upstart.
func (h *ZookeeperHealer) Heal() error {
	if h.needsHeal() {
		bootstrapMachine := getBootstrapMachine()
		log.Printf("Healing zookeeper")
		upStartCmd("stop", "zookeeper", bootstrapMachine.IpAddress)
		return upStartCmd("start", "zookeeper", bootstrapMachine.IpAddress)
	}
	log.Printf("Zookeeper needs no cure, skipping...")
	return nil
}

// BootstrapProvisionHealer is an import for the Healer interface. For more
// details on how a healer work, check the documentation of the heal package.
type BootstrapProvisionHealer struct{}

// Heal starts the juju-provision-agent using upstart.
func (h *BootstrapProvisionHealer) Heal() error {
	bootstrapMachine := getBootstrapMachine()
	log.Printf("Healing bootstrap juju-provision-agent")
	return upStartCmd("start", "juju-provision-agent", bootstrapMachine.IpAddress)
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

// needsHeal returns true if the AgentState of bootstrap machine is "not-started".
func (h *BootstrapMachineHealer) needsHeal() bool {
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
	if h.needsHeal() {
		bootstrapMachine := getBootstrapMachine()
		log.Printf("Healing bootstrap juju-machine-agent")
		upStartCmd("stop", "juju-machine-agent", bootstrapMachine.IpAddress)
		return upStartCmd("start", "juju-machine-agent", bootstrapMachine.IpAddress)
	}
	log.Printf("Bootstrap juju-machine-agent needs no cure, skipping...")
	return nil
}
