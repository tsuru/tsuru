// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/repository"
	"io"
	"launchpad.net/goyaml"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	provision.Register("juju", &JujuProvisioner{})
}

// Sometimes juju gives the "no node" error when destroying a service or
// removing a unit. This is one of Zookeeper bad behaviour. This constant
// indicates how many times JujuProvisioner will call destroy-service and
// remove-unit before raising the error.
const destroyTries = 5

// JujuProvisioner is an implementation for the Provisioner interface. For more
// details on how a provisioner work, check the documentation of the provision
// package.
type JujuProvisioner struct {
	elb *bool
}

func (p *JujuProvisioner) elbSupport() bool {
	if p.elb == nil {
		elb, _ := config.GetBool("juju:use-elb")
		p.elb = &elb
	}
	return *p.elb
}

func (p *JujuProvisioner) enqueueUnits(app string, units ...string) {
	args := make([]string, len(units)+1)
	args[0] = app
	for i := range units {
		args[i+1] = units[i]
	}
	enqueue(&queue.Message{
		Action: addUnitToLoadBalancer,
		Args:   args,
	})
}

func (p *JujuProvisioner) Provision(app provision.App) error {
	var buf bytes.Buffer
	args := []string{
		"deploy", "--repository", "/home/charms",
		"local:" + app.GetFramework(), app.GetName(),
	}
	err := runCmd(true, &buf, &buf, args...)
	out := buf.String()
	if err != nil {
		app.Log("Failed to create machine: "+out, "tsuru")
		return &provision.Error{Reason: out, Err: err}
	}
	if p.elbSupport() {
		if err = p.LoadBalancer().Create(app); err != nil {
			return err
		}
		p.enqueueUnits(app.GetName())
	}
	return nil
}

func (p *JujuProvisioner) destroyService(app provision.App) error {
	var (
		err error
		buf bytes.Buffer
		out string
	)
	// Sometimes juju gives the "no node" error. This is one of Zookeeper
	// bad behaviors. Let's try it multiple times before raising the error
	// to the user, and hope that someday we run away from Zookeeper.
	for i := 0; i < destroyTries; i++ {
		buf.Reset()
		err = runCmd(false, &buf, &buf, "destroy-service", app.GetName())
		if err == nil {
			break
		}
		out = buf.String()
	}
	if err != nil {
		msg := fmt.Sprintf("Failed to destroy the app: %s.", out)
		app.Log(msg, "tsuru")
		return &provision.Error{Reason: out, Err: err}
	}
	return nil
}

func (p *JujuProvisioner) terminateMachines(app provision.App, units ...provision.AppUnit) error {
	var buf bytes.Buffer
	if len(units) < 1 {
		units = app.ProvisionUnits()
	}
	for _, u := range units {
		buf.Reset()
		err := runCmd(false, &buf, &buf, "terminate-machine", strconv.Itoa(u.GetMachine()))
		out := buf.String()
		if err != nil {
			msg := fmt.Sprintf("Failed to destroy unit %s: %s", u.GetName(), out)
			app.Log(msg, "tsuru")
			return &provision.Error{Reason: out, Err: err}
		}
	}
	return nil
}

func (p *JujuProvisioner) Destroy(app provision.App) error {
	var err error
	if err = p.destroyService(app); err != nil {
		return err
	}
	if p.elbSupport() {
		err = p.LoadBalancer().Destroy(app)
	}
	go p.terminateMachines(app)
	return err
}

func (p *JujuProvisioner) AddUnits(app provision.App, n uint) ([]provision.Unit, error) {
	if n < 1 {
		return nil, errors.New("Cannot add zero units.")
	}
	var (
		buf   bytes.Buffer
		units []provision.Unit
	)
	err := runCmd(true, &buf, &buf, "set", app.GetName(), "app-repo="+repository.GetReadOnlyUrl(app.GetName()))
	if err != nil {
		return nil, &provision.Error{Reason: buf.String(), Err: err}
	}
	buf.Reset()
	err = runCmd(false, &buf, &buf, "add-unit", app.GetName(), "--num-units", strconv.FormatUint(uint64(n), 10))
	if err != nil {
		return nil, &provision.Error{Reason: buf.String(), Err: err}
	}
	unitRe := regexp.MustCompile(fmt.Sprintf(
		`Unit '(%s/\d+)' added to service '%s'`, app.GetName(), app.GetName()),
	)
	reader := bufio.NewReader(&buf)
	line, err := reader.ReadString('\n')
	names := make([]string, n)
	units = make([]provision.Unit, n)
	i := 0
	for err == nil {
		matches := unitRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			units[i] = provision.Unit{Name: matches[1]}
			names[i] = matches[1]
			i++
		}
		line, err = reader.ReadString('\n')
	}
	if err != io.EOF {
		return nil, &provision.Error{Reason: buf.String(), Err: err}
	}
	p.enqueueUnits(app.GetName(), names...)
	return units, nil
}

func (p *JujuProvisioner) removeUnits(app provision.App, units ...provision.AppUnit) error {
	var (
		buf bytes.Buffer
		err error
	)
	cmd := make([]string, len(units)+1)
	cmd[0] = "remove-unit"
	for i, unit := range units {
		cmd[i+1] = unit.GetName()
	}
	// Sometimes juju gives the "no node" error. This is one of Zookeeper bad
	// behaviors. Let's try it multiple times before raising the error to the
	// user, and hope that someday we run away from Zookeeper.
	for i := 0; i < destroyTries; i++ {
		buf.Reset()
		err = runCmd(true, &buf, &buf, cmd...)
		if err == nil {
			break
		}
	}
	if err != nil {
		return &provision.Error{Reason: buf.String(), Err: err}
	}
	go p.terminateMachines(app, units...)
	return nil
}

func (p *JujuProvisioner) RemoveUnit(app provision.App, name string) error {
	var unit provision.AppUnit
	for _, unit = range app.ProvisionUnits() {
		if unit.GetName() == name {
			break
		}
	}
	if unit.GetName() != name {
		return fmt.Errorf("App %q does not have a unit named %q.", app.GetName(), name)
	}
	return p.removeUnits(app, unit)
}

func (p *JujuProvisioner) RemoveUnits(app provision.App, n uint) ([]int, error) {
	units := app.ProvisionUnits()
	length := uint(len(units))
	if length == n {
		return nil, errors.New("You can't remove all units from an app.")
	} else if length < n {
		return nil, fmt.Errorf("You can't remove %d units from this app because it has only %d units.", n, length)
	}
	result := make([]int, n)
	if err := p.removeUnits(app, units[:n]...); err != nil {
		return nil, err
	}
	for i := 0; i < len(result); i++ {
		result[i] = i
	}
	return result, nil
}

func (p *JujuProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	arguments := []string{"ssh", "-o", "StrictHostKeyChecking no", "-q"}
	units := app.ProvisionUnits()
	length := len(units)
	for i, unit := range units {
		if length > 1 {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stdout, "Output from unit %q:\n\n", unit.GetName())
			if status := unit.GetStatus(); status != provision.StatusStarted {
				fmt.Fprintf(stdout, "Unit state is %q, it must be %q for running commands.\n",
					status, provision.StatusStarted)
				continue
			}
		}
		var cmdargs []string
		cmdargs = append(cmdargs, arguments...)
		cmdargs = append(cmdargs, strconv.Itoa(unit.GetMachine()), cmd)
		cmdargs = append(cmdargs, args...)
		err := runCmd(true, stdout, stderr, cmdargs...)
		fmt.Fprintln(stdout)
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
		for unitName, u := range service.Units {
			machine := out.Machines[u.Machine]
			unit := provision.Unit{
				Name:       unitName,
				AppName:    name,
				Machine:    u.Machine,
				InstanceId: machine.InstanceId,
				Ip:         machine.IpAddress,
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

func (p *JujuProvisioner) Addr(app provision.App) (string, error) {
	if p.elbSupport() {
		return p.LoadBalancer().Addr(app)
	}
	units := app.ProvisionUnits()
	if len(units) < 1 {
		return "", fmt.Errorf("App %q has no units.", app.GetName())
	}
	return units[0].GetIp(), nil
}

func (p *JujuProvisioner) LoadBalancer() *ELBManager {
	if p.elbSupport() {
		return &ELBManager{}
	}
	return nil
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

func runCmd(filter bool, stdout, stderr io.Writer, cmd ...string) error {
	if filter {
		stdout = &Writer{stdout}
		stderr = &Writer{stderr}
	}
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
	command.Stdout = &Writer{&buf}
	command.Stderr = &Writer{&buf}
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

func unitStatus(instanceState, agentState, machineAgentState string) provision.Status {
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
