// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bufio"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/heal"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"net"
	"os/exec"
	"strings"
)

func init() {
	heal.Register("bootstrap", bootstrapMachineHealer{})
	heal.Register("bootstrap-provision", bootstrapProvisionHealer{})
	heal.Register("instance-machine", instanceMachineHealer{})
	heal.Register("instance-agents-config", instanceAgentsConfigHealer{})
	heal.Register("instance-unit", instanceUnitHealer{})
	heal.Register("zookeeper", zookeeperHealer{})
	heal.Register("elb-instance", elbInstanceHealer{})
}

// instanceAgentsConfigHealer is an implementation for the Haler interface. For more
// detail on how a healer work, check the documentation of the heal package.
type instanceAgentsConfigHealer struct {
	e *ec2.EC2
}

func (h *instanceAgentsConfigHealer) ec2() *ec2.EC2 {
	if h.e == nil {
		h.e = getEC2Endpoint()
	}
	return h.e
}

func getEC2Endpoint() *ec2.EC2 {
	access, err := config.GetString("aws:access-key-id")
	if err != nil {
		log.Fatal(err)
	}
	secret, err := config.GetString("aws:secret-access-key")
	if err != nil {
		log.Fatal(err)
	}
	auth := aws.Auth{AccessKey: access, SecretKey: secret}
	return ec2.New(auth, aws.Region{})
}

func (h *instanceAgentsConfigHealer) getPrivateDns(instanceId string) (string, error) {
	resp, err := h.ec2().Instances([]string{instanceId}, nil)
	if err != nil {
		return "", err
	}
	dns := resp.Reservations[0].Instances[0].PrivateDNSName
	return dns, nil
}

func (instanceAgentsConfigHealer) Heal() error {
	return nil
}

// InstanceUnitHealer is an implementation for the Healer interface. For more
// detail on how a healer work, check the documentation of the heal package.
type instanceUnitHealer struct{}

// Heal iterates through all juju units verifying if
// a juju-unit-agent is down and heal these machines.
func (h instanceUnitHealer) Heal() error {
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
func (h instanceMachineHealer) Heal() error {
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
func (h zookeeperHealer) needsHeal() bool {
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
func (h zookeeperHealer) Heal() error {
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
func (h bootstrapProvisionHealer) Heal() error {
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
func (h bootstrapMachineHealer) needsHeal() bool {
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
func (h bootstrapMachineHealer) Heal() error {
	if h.needsHeal() {
		bootstrapMachine := getBootstrapMachine()
		log.Printf("Healing bootstrap juju-machine-agent")
		upStartCmd("stop", "juju-machine-agent", bootstrapMachine.IpAddress)
		return upStartCmd("start", "juju-machine-agent", bootstrapMachine.IpAddress)
	}
	log.Printf("Bootstrap juju-machine-agent needs no cure, skipping...")
	return nil
}

type elbInstanceHealer struct{}

func (h elbInstanceHealer) Heal() error {
	apps := h.getUnhealthyApps()
	if len(apps) == 0 {
		log.Print("No app is down.")
		return nil
	}
	names := make([]string, len(apps))
	i := 0
	for n := range apps {
		names[i] = n
		i++
	}
	if instances, err := h.checkInstances(names); err == nil && len(instances) > 0 {
		for _, instance := range instances {
			a := apps[instance.lb]
			if err := a.RemoveUnit(instance.id); err != nil {
				return err
			}
			if err := a.AddUnits(1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h elbInstanceHealer) checkInstances(names []string) ([]elbInstance, error) {
	if elbSupport, _ := config.GetBool("juju:use-elb"); !elbSupport {
		return nil, nil
	}
	lbs, err := h.describeLoadBalancers(names)
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

func (h elbInstanceHealer) getUnhealthyApps() map[string]app.App {
	conn, err := db.Conn()
	if err != nil {
		return nil
	}
	var all []app.App
	apps := make(map[string]app.App)
	s := map[string]interface{}{"name": 1, "units": 1}
	err = conn.Apps().Find(nil).Select(s).All(&all)
	if err != nil {
		return nil
	}
	for _, a := range all {
		for _, u := range a.ProvisionUnits() {
			if u.GetStatus() == provision.StatusDown ||
				u.GetStatus() == provision.StatusError {
				apps[a.Name] = a
				break
			}
		}
	}
	return apps
}

func (h elbInstanceHealer) describeLoadBalancers(names []string) ([]string, error) {
	resp, err := getELBEndpoint().DescribeLoadBalancers(names...)
	if err != nil {
		return nil, err
	}
	lbs := make([]string, len(resp.LoadBalancerDescriptions))
	for i, lbdesc := range resp.LoadBalancerDescriptions {
		lbs[i] = lbdesc.LoadBalancerName
	}
	return lbs, nil
}

func (h elbInstanceHealer) describeInstancesHealth(lb string) ([]elbInstance, error) {
	resp, err := getELBEndpoint().DescribeInstanceHealth(lb)
	if err != nil {
		return nil, err
	}
	instances := make([]elbInstance, len(resp.InstanceStates))
	for i, state := range resp.InstanceStates {
		instances[i].id = state.InstanceId
		instances[i].description = state.Description
		instances[i].reasonCode = state.ReasonCode
		instances[i].state = state.State
		instances[i].lb = lb
	}
	return instances, nil
}
