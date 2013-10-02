// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	_ "github.com/globocom/tsuru/router/elb"
	"launchpad.net/goyaml"
	"net/http"
	"regexp"
	"strings"
)

func (p *JujuProvisioner) getOutput() (jujuOutput, error) {
	output, err := execWithTimeout(30e9, "juju", "status")
	if err != nil {
		return jujuOutput{}, cmdError(string(output), err, []string{"juju", "status"})
	}
	var out jujuOutput
	err = goyaml.Unmarshal(output, &out)
	if err != nil {
		reason := fmt.Sprintf("%q returned invalid data: %s", "juju status", output)
		return jujuOutput{}, &provision.Error{Reason: reason, Err: err}
	}
	return out, nil
}

func (p *JujuProvisioner) saveBootstrapMachine(m machine) error {
	collection := p.bootstrapCollection()
	defer collection.Close()
	_, err := collection.Upsert(nil, &m)
	return err
}

func (p *JujuProvisioner) bootstrapCollection() *db.Collection {
	name, err := config.GetString("juju:bootstrap-collection")
	if err != nil {
		log.Fatalf("FATAL: %s.", err)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Fatalf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
}

func (p *JujuProvisioner) collectStatus() ([]provision.Unit, error) {
	out, err := p.getOutput()
	if err != nil {
		return nil, err
	}
	var units []provision.Unit
	for name, service := range out.Services {
		for unitName, u := range service.Units {
			machine := out.Machines[u.Machine]
			unit := provision.Unit{
				Name:       unitName,
				AppName:    name,
				Machine:    u.Machine,
				InstanceId: machine.InstanceID,
				Ip:         machine.IPAddress,
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
	p.saveBootstrapMachine(out.Machines[0])
	return units, err
}

func (p *JujuProvisioner) CollectStatus() ([]provision.Unit, error) {
	units, err := p.collectStatus()
	if err != nil {
		return nil, err
	}
	go p.heal(units)
	return units, err
}

func unitStatus(instanceState, agentState, machineAgentState string) provision.Status {
	if instanceState == "error" ||
		machineAgentState == "start-error" ||
		strings.Contains(agentState, "error") {
		return provision.StatusDown
	}
	if machineAgentState == "pending" || machineAgentState == "not-started" || machineAgentState == "" {
		return provision.StatusBuilding
	}
	if instanceState == "pending" || instanceState == "" {
		return provision.StatusBuilding
	}
	if agentState == "down" {
		return provision.StatusDown
	}
	if machineAgentState == "running" && agentState == "not-started" {
		return provision.StatusBuilding
	}
	if machineAgentState == "running" && instanceState == "running" && agentState == "pending" {
		return provision.StatusBuilding
	}
	if machineAgentState == "running" && agentState == "started" && instanceState == "running" {
		return provision.StatusStarted
	}
	return provision.StatusBuilding
}

// isReachable returns true if the web application deploy in the
// unit is accessible via http in the port 80.
func IsReachable(unit provision.AppUnit) (bool, error) {
	url := fmt.Sprintf("http://%s", unit.GetIp())
	response, err := http.Get(url)
	if err != nil {
		return false, err
	}
	if response.StatusCode == http.StatusBadGateway {
		return false, nil
	}
	return true, nil
}
