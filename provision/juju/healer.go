// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bufio"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/heal"
	"github.com/globocom/tsuru/log"
	"net"
	"os/exec"
	"strings"
)

func init() {
	heal.Register("bootstrap", &bootstrapMachineHealer{})
	heal.Register("bootstrap-provision", &bootstrapProvisionHealer{})
	heal.Register("instance-machine", &instanceMachineHealer{})
	heal.Register("instance-unit", &instanceUnitHealer{})
	heal.Register("zookeeper", &zookeeperHealer{})
	heal.Register("elb-instance", ELBInstanceHealer{})
}

// InstanceUnitHealer is an implementation for the Healer interface. For more
// detail on how a healer work, check the documentation of the heal package.
type instanceUnitHealer struct{}

// Heal iterates through all juju units verifying if
// a juju-unit-agent is down and heal these machines.
func (h *instanceUnitHealer) Heal() error {
	p := JujuProvisioner{}
	output, _ := p.getOutput()
	for _, service := range output.Services {
		for unitName, unit := range service.Units {
			agent := fmt.Sprintf("juju-%s", strings.Join(strings.Split(unitName, "/"), "-"))
			if unit.AgentState == "down" {
				log.Printf("Healing %s", agent)
				upStartCmd("stop", agent, unit.PublicAddress)
				upStartCmd("start", agent, unit.PublicAddress)
			} else {
				log.Printf("%s needs no cure, skipping...", agent)
			}
		}
	}
	return nil
}

// InstanceMachineHealer is an implementation for the Healer interface. For more
// detail on how a healer work, check the documentation of the heal package.
type instanceMachineHealer struct{}

// Heal iterates through all juju machines verifying if
// a juju-machine-agent is down and heal these machines.
func (h *instanceMachineHealer) Heal() error {
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
type zookeeperHealer struct{}

// needsHeal verifies if zookeeper is ok using 'ruok' command.
func (h *zookeeperHealer) needsHeal() bool {
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
func (h *zookeeperHealer) Heal() error {
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
type bootstrapProvisionHealer struct{}

// Heal starts the juju-provision-agent using upstart.
func (h *bootstrapProvisionHealer) Heal() error {
	bootstrapMachine := getBootstrapMachine()
	log.Printf("Healing bootstrap juju-provision-agent")
	return upStartCmd("start", "juju-provision-agent", bootstrapMachine.IpAddress)
}

// BootstrapMachineHealer is an implementation for the Healer interface. For more
// details on how a healer work, check the documentation of the heal package.
type bootstrapMachineHealer struct{}

// getBootstrapMachine returns the bootstrap machine.
func getBootstrapMachine() machine {
	p := JujuProvisioner{}
	output, _ := p.getOutput()
	// for juju bootstrap machine always is the machine 0.
	return output.Machines[0]
}

// needsHeal returns true if the AgentState of bootstrap machine is "not-started".
func (h *bootstrapMachineHealer) needsHeal() bool {
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
func (h *bootstrapMachineHealer) Heal() error {
	if h.needsHeal() {
		bootstrapMachine := getBootstrapMachine()
		log.Printf("Healing bootstrap juju-machine-agent")
		upStartCmd("stop", "juju-machine-agent", bootstrapMachine.IpAddress)
		return upStartCmd("start", "juju-machine-agent", bootstrapMachine.IpAddress)
	}
	log.Printf("Bootstrap juju-machine-agent needs no cure, skipping...")
	return nil
}

type ELBInstanceHealer struct{}

func (h ELBInstanceHealer) Heal() error {
	if instances, err := h.checkInstances(); err == nil && len(instances) > 0 {
		for _, instance := range instances {
			app := app.App{Name: instance.lb}
			if err := app.Get(); err != nil {
				log.Printf("Warning: app not found for the load balancer %s.", instance.lb)
				continue
			}
			if err := app.RemoveUnit(instance.instanceId); err != nil {
				return err
			}
			if err := app.AddUnits(1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h ELBInstanceHealer) checkInstances() ([]elbInstance, error) {
	if elbSupport, _ := config.GetBool("juju:use-elb"); !elbSupport {
		return nil, nil
	}
	lbs, err := h.describeLoadBalancers()
	if err != nil {
		return nil, err
	}
	var unhealthy []elbInstance
	description := "Instance has failed at least the UnhealthyThreshold number of health checks consecutively."
	state := "OutOfService"
	reasonCode := "Instance"
	for _, lb := range lbs {
		instances, err := h.describeInstancesHealth(lb)
		if err != nil {
			return nil, err
		}
		for _, instance := range instances {
			if instance.description == description &&
				instance.state == state &&
				instance.reasonCode == reasonCode {
				unhealthy = append(unhealthy, instance)
			}
		}
	}
	log.Printf("Found %d unhealthy instances.", len(unhealthy))
	return unhealthy, nil
}

func (h ELBInstanceHealer) describeLoadBalancers() ([]string, error) {
	resp, err := getELBEndpoint().DescribeLoadBalancers()
	if err != nil {
		return nil, err
	}
	lbs := make([]string, len(resp.LoadBalancerDescriptions))
	for i, lbdesc := range resp.LoadBalancerDescriptions {
		lbs[i] = lbdesc.LoadBalancerName
	}
	return lbs, nil
}

func (h ELBInstanceHealer) describeInstancesHealth(lb string) ([]elbInstance, error) {
	resp, err := getELBEndpoint().DescribeInstanceHealth(lb)
	if err != nil {
		return nil, err
	}
	instances := make([]elbInstance, len(resp.InstanceStates))
	for i, state := range resp.InstanceStates {
		instances[i].instanceId = state.InstanceId
		instances[i].description = state.Description
		instances[i].reasonCode = state.ReasonCode
		instances[i].state = state.State
		instances[i].lb = lb
	}
	return instances, nil
}
