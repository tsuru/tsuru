// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bufio"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/heal"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"net"
	"strings"
)

func init() {
	heal.Register("juju", "bootstrap", bootstrapMachineHealer{})
	heal.Register("juju", "bootstrap-provision", bootstrapProvisionHealer{})
	heal.Register("juju", "instance-machine", instanceMachineHealer{})
	heal.Register("juju", "instance-agents-config", instanceAgentsConfigHealer{})
	heal.Register("juju", "instance-unit", instanceUnitHealer{})
	heal.Register("juju", "zookeeper", zookeeperHealer{})
	heal.Register("juju", "elb-instance", elbInstanceHealer{})
	heal.Register("juju", "bootstrap-instanceid", bootstrapInstanceIDHealer{})
}

type bootstrapInstanceIDHealer struct {
	s *s3.S3
	e *ec2.EC2
}

func (h bootstrapInstanceIDHealer) Heal() error {
	if h.needsHeal() {
		log.Debug("healing bootstrap instance id")
		jujuBucket, err := config.GetString("juju:bucket")
		if err != nil {
			return err
		}
		bucket := h.s3().Bucket(jujuBucket)
		ec2InstanceID, err := h.bootstrapInstanceID()
		if err != nil {
			return err
		}
		return bucket.Put("provider-state", []byte("zookeeper-instances: ["+ec2InstanceID+"]"), "binary/octet-stream", s3.BucketOwnerFull)
	}
	return nil
}

func (h *bootstrapInstanceIDHealer) needsHeal() bool {
	s3InstanceID, err := h.bootstrapInstanceIDFromBucket()
	if err != nil {
		return false
	}
	ec2InstanceID, err := h.bootstrapInstanceID()
	if err != nil {
		return false
	}
	if s3InstanceID != ec2InstanceID {
		return true
	}
	return false
}

func (h *bootstrapInstanceIDHealer) ec2() *ec2.EC2 {
	if h.e == nil {
		h.e = getEC2Endpoint()
	}
	return h.e
}

func (h *bootstrapInstanceIDHealer) s3() *s3.S3 {
	if h.s == nil {
		h.s = h.getS3Endpoint()
	}
	return h.s
}

func (bootstrapInstanceIDHealer) getS3Endpoint() *s3.S3 {
	access, err := config.GetString("aws:access-key-id")
	if err != nil {
		log.Fatal(err.Error())
	}
	secret, err := config.GetString("aws:secret-access-key")
	if err != nil {
		log.Fatal(err.Error())
	}
	auth := aws.Auth{AccessKey: access, SecretKey: secret}
	return s3.New(auth, aws.USEast)
}

func (h *bootstrapInstanceIDHealer) bootstrapInstanceIDFromBucket() (string, error) {
	jujuBucket, err := config.GetString("juju:bucket")
	if err != nil {
		return "", err
	}
	bucket := h.s3().Bucket(jujuBucket)
	data, err := bucket.Get("provider-state")
	if err != nil {
		return "", err
	}
	s := strings.Replace(string(data), "zookeeper-instances: [", "", -1)
	s = strings.Replace(s, "]", "", -1)
	return s, nil
}

func (h *bootstrapInstanceIDHealer) bootstrapInstanceID() (string, error) {
	resp, err := h.ec2().Instances(nil, nil)
	if err != nil {
		return "", err
	}
	for _, reservation := range resp.Reservations {
		for _, group := range reservation.SecurityGroups {
			if group.Name == "juju-delta-0" {
				for _, instance := range reservation.Instances {
					return instance.InstanceId, nil
				}
			}
		}
	}
	return "", nil
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
		log.Fatal(err.Error())
	}
	secret, err := config.GetString("aws:secret-access-key")
	if err != nil {
		log.Fatal(err.Error())
	}
	endpoint, err := config.GetString("aws:ec2:endpoint")
	if err != nil {
		log.Fatal(err.Error())
	}
	auth := aws.Auth{AccessKey: access, SecretKey: secret}
	return ec2.New(auth, aws.Region{EC2Endpoint: endpoint})
}

// getPrivateDns returns the private dns for an instance.
func (h *instanceAgentsConfigHealer) getPrivateDns(instanceId string) (string, error) {
	log.Debugf("getting dns for %s", instanceId)
	resp, err := h.ec2().Instances([]string{instanceId}, nil)
	if err != nil {
		log.Errorf("error in gettings dns for %s", instanceId)
		log.Error(err.Error())
		return "", err
	}
	dns := resp.Reservations[0].Instances[0].PrivateDNSName
	return dns, nil
}

// bootstrapProvisionHealer returns the bootstrap private dns.
func (h *instanceAgentsConfigHealer) bootstrapPrivateDns() (string, error) {
	machine := getBootstrapMachine()
	return h.getPrivateDns(machine.InstanceID)
}

// Heal verifies if the bootstrap private dns is different of the bootstrap
// private dns setted into agents for each machine.
// If the bootstrap private dns is wrong, Heal will injects the correct value.
func (h instanceAgentsConfigHealer) Heal() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var apps []app.App
	err = conn.Apps().Find(nil).All(&apps)
	if err != nil {
		return err
	}
	dns, err := h.bootstrapPrivateDns()
	if err != nil {
		return err
	}
	for _, app := range apps {
		for _, u := range app.ProvisionedUnits() {
			args := []string{"-o", "StrictHostKeyChecking no", "-q", "-l", "ubuntu", u.GetIp(), "grep", dns, "/etc/init/juju-machine-agent.conf"}
			err := executor().Execute("ssh", args, nil, nil, nil)
			if err != nil {
				log.Debugf("Injecting bootstrap private dns for machine %d", u.GetMachine())
				args = []string{"-o", "StrictHostKeyChecking no", "-q", "-l", "ubuntu", u.GetIp(), "sudo", "sed", "-i", "'s/env JUJU_ZOOKEEPER=.*/env JUJU_ZOOKEEPER=\"" + dns + ":2181\"/g'", "/etc/init/juju-machine-agent.conf"}
				executor().Execute("ssh", args, nil, nil, nil)
			}
			agent := fmt.Sprintf("/etc/init/juju-%s.conf", strings.Join(strings.Split(u.GetName(), "/"), "-"))
			args = []string{"-o", "StrictHostKeyChecking no", "-q", "-l", "ubuntu", u.GetIp(), "grep", dns, agent}
			err = executor().Execute("ssh", args, nil, nil, nil)
			if err != nil {
				log.Debugf("Injecting bootstrap private dns for agent %s", agent)
				args = []string{"-o", "StrictHostKeyChecking no", "-q", "-l", "ubuntu", u.GetIp(), "sudo", "sed", "-i", "'s/env JUJU_ZOOKEEPER=.*/env JUJU_ZOOKEEPER=\"" + dns + ":2181\"/g'", agent}
				executor().Execute("ssh", args, nil, nil, nil)
			}
		}
	}
	return nil
}

// InstanceUnitHealer is an implementation for the Healer interface. For more
// detail on how a healer work, check the documentation of the heal package.
type instanceUnitHealer struct{}

// Heal iterates through all juju units verifying if
// a juju-unit-agent is down and heal these machines.
func (h instanceUnitHealer) Heal() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var apps []app.App
	err = conn.Apps().Find(nil).All(&apps)
	if err != nil {
		return err
	}
	for _, app := range apps {
		for _, u := range app.ProvisionedUnits() {
			agent := fmt.Sprintf("juju-%s", strings.Join(strings.Split(u.GetName(), "/"), "-"))
			if u.GetStatus() == provision.StatusDown {
				log.Debugf("Healing %s", agent)
				upStartCmd("stop", agent, u.GetIp())
				upStartCmd("start", agent, u.GetIp())
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
			log.Debugf("Healing juju-machine-agent in machine %s", machine.InstanceID)
			upStartCmd("stop", "juju-machine-agent", machine.IPAddress)
			upStartCmd("start", "juju-machine-agent", machine.IPAddress)
		} else {
			log.Debugf("juju-machine-agent for machine %s needs no cure, skipping...", machine.InstanceID)
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
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:2181", bootstrapMachine.IPAddress))
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
		log.Debug("Healing zookeeper")
		upStartCmd("stop", "zookeeper", bootstrapMachine.IPAddress)
		return upStartCmd("start", "zookeeper", bootstrapMachine.IPAddress)
	}
	log.Debug("Zookeeper needs no cure, skipping...")
	return nil
}

// BootstrapProvisionHealer is an import for the Healer interface. For more
// details on how a healer work, check the documentation of the heal package.
type bootstrapProvisionHealer struct{}

// Heal starts the juju-provision-agent using upstart.
func (h bootstrapProvisionHealer) Heal() error {
	bootstrapMachine := getBootstrapMachine()
	log.Debug("Healing bootstrap juju-provision-agent")
	return upStartCmd("start", "juju-provision-agent", bootstrapMachine.IPAddress)
}

// BootstrapMachineHealer is an implementation for the Healer interface. For more
// details on how a healer work, check the documentation of the heal package.
type bootstrapMachineHealer struct{}

// getBootstrapMachine returns the bootstrap machine.
func getBootstrapMachine() machine {
	var b machine
	p := JujuProvisioner{}
	collection := p.bootstrapCollection()
	defer collection.Close()
	collection.Find(nil).One(&b)
	return b
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
	log.Debug(strings.Join(args, " "))
	return executor().Execute("ssh", args, nil, nil, nil)
}

// Heal executes the action for heal the bootstrap machine agent.
func (h bootstrapMachineHealer) Heal() error {
	if h.needsHeal() {
		bootstrapMachine := getBootstrapMachine()
		log.Debug("Healing bootstrap juju-machine-agent")
		upStartCmd("stop", "juju-machine-agent", bootstrapMachine.IPAddress)
		return upStartCmd("start", "juju-machine-agent", bootstrapMachine.IPAddress)
	}
	log.Debug("Bootstrap juju-machine-agent needs no cure, skipping...")
	return nil
}

type elbInstanceHealer struct{}

func (h elbInstanceHealer) Heal() error {
	apps := h.getUnhealthyApps()
	if len(apps) == 0 {
		log.Debug("No app is down.")
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
	log.Debugf("Found %d unhealthy instances.", len(unhealthy))
	return unhealthy, nil
}

func (h elbInstanceHealer) getUnhealthyApps() map[string]app.App {
	conn, err := db.Conn()
	if err != nil {
		return nil
	}
	defer conn.Close()
	var all []app.App
	apps := make(map[string]app.App)
	s := map[string]interface{}{"name": 1, "units": 1}
	err = conn.Apps().Find(nil).Select(s).All(&all)
	if err != nil {
		return nil
	}
	for _, a := range all {
		for _, u := range a.ProvisionedUnits() {
			if u.GetStatus() == provision.StatusDown {
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
