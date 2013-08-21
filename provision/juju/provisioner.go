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
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/deploy"
	"github.com/globocom/tsuru/exec"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/router"
	_ "github.com/globocom/tsuru/router/elb"
	"github.com/globocom/tsuru/safe"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	osexec "os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	provision.Register("juju", &JujuProvisioner{})
}

var execut exec.Executor
var execMut sync.RWMutex

func executor() exec.Executor {
	execMut.RLock()
	defer execMut.RUnlock()
	if execut == nil {
		execMut.RUnlock()
		execMut.Lock()
		execut = exec.OsExecutor{}
		execMut.Unlock()
		execMut.RLock()
	}
	return execut
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

func Router() (router.Router, error) {
	return router.Get("elb")
}

func (p *JujuProvisioner) unitsCollection() (*db.Storage, *mgo.Collection) {
	name, err := config.GetString("juju:units-collection")
	if err != nil {
		log.Fatalf("FATAL: %s.", err)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Fatalf("Failed to connect to the database: %s", err)
	}
	return conn, conn.Collection(name)
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
	charms, err := config.GetString("juju:charms-path")
	if err != nil {
		return errors.New(`Setting "juju:charms-path" is not defined.`)
	}
	args := []string{
		"deploy", "--repository", charms,
		"local:" + app.GetPlatform(), app.GetName(),
	}
	err = runCmd(false, &buf, &buf, args...)
	out := buf.String()
	if err != nil {
		app.Log("Failed to create machine: "+out, "tsuru")
		return cmdError(out, err, args)
	}
	setOption := []string{
		"set", app.GetName(), "app-repo=" + repository.ReadOnlyURL(app.GetName()),
	}
	runCmd(true, &buf, &buf, setOption...)
	if p.elbSupport() {
		router, err := Router()
		if err != nil {
			return err
		}
		if err = router.AddBackend(app.GetName()); err != nil {
			return err
		}
		p.enqueueUnits(app.GetName())
	}
	return nil
}

func (p *JujuProvisioner) Restart(app provision.App) error {
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "/var/lib/tsuru/hooks/restart")
	if err != nil {
		msg := fmt.Sprintf("Failed to restart the app (%s): %s", err, buf.String())
		app.Log(msg, "tsuru-provisioner")
		return &provision.Error{Reason: buf.String(), Err: err}
	}
	return nil
}

func (JujuProvisioner) Swap(app1, app2 provision.App) error {
	r, err := Router()
	if err != nil {
		log.Printf("Failed to get router: %s", err.Error())
		return err
	}
	app1Routes, err := r.Routes(app1.GetName())
	if err != nil {
		return err
	}
	app2Routes, err := r.Routes(app2.GetName())
	if err != nil {
		return err
	}
	for _, route := range app1Routes {
		err = r.AddRoute(app2.GetName(), route)
		if err != nil {
			return err
		}
		err = r.RemoveRoute(app1.GetName(), route)
		if err != nil {
			return err
		}
	}
	for _, route := range app2Routes {
		err = r.AddRoute(app1.GetName(), route)
		if err != nil {
			return err
		}
		err = r.RemoveRoute(app2.GetName(), route)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *JujuProvisioner) Deploy(a provision.App, version string, w io.Writer) error {
	var buf bytes.Buffer
	setOption := []string{"set", a.GetName(), "app-version=" + version}
	if err := runCmd(true, &buf, &buf, setOption...); err != nil {
		log.Printf("juju: Failed to set app-version. Error: %s.\nCommand output: %s", err, &buf)
	}
	return deploy.Git(p, a, version, w)
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
		return cmdError(out, err, []string{"destroy-service", app.GetName()})
	}
	return nil
}

func (p *JujuProvisioner) terminateMachines(app provision.App, units ...provision.AppUnit) error {
	var buf bytes.Buffer
	if len(units) < 1 {
		units = app.ProvisionedUnits()
	}
	for _, u := range units {
		buf.Reset()
		err := runCmd(false, &buf, &buf, "terminate-machine", strconv.Itoa(u.GetMachine()))
		out := buf.String()
		if err != nil {
			msg := fmt.Sprintf("Failed to destroy unit %s: %s", u.GetName(), out)
			app.Log(msg, "tsuru")
			log.Printf("Failed to destroy unit %q from the app %q: %s", u.GetName(), app.GetName(), out)
			return cmdError(out, err, []string{"terminate-machine", strconv.Itoa(u.GetMachine())})
		}
	}
	return nil
}

func (p *JujuProvisioner) deleteUnits(app provision.App) {
	units := app.ProvisionedUnits()
	names := make([]string, len(units))
	for i, u := range units {
		names[i] = u.GetName()
	}
	conn, collection := p.unitsCollection()
	defer conn.Close()
	collection.RemoveAll(bson.M{"_id": bson.M{"$in": names}})
}

func (p *JujuProvisioner) Destroy(app provision.App) error {
	var err error
	if err = p.destroyService(app); err != nil {
		return err
	}
	if p.elbSupport() {
		router, err := Router()
		if err != nil {
			return err
		}
		err = router.RemoveBackend(app.GetName())
	}
	go p.terminateMachines(app)
	p.deleteUnits(app)
	return err
}

func setOption(serviceName, key, value string) error {
	var buf bytes.Buffer
	args := []string{"set", serviceName, key + "=" + value}
	err := runCmd(false, &buf, &buf, args...)
	if err != nil {
		return cmdError(buf.String(), err, args)
	}
	return nil
}

func (p *JujuProvisioner) AddUnits(app provision.App, n uint) ([]provision.Unit, error) {
	if n < 1 {
		return nil, errors.New("Cannot add zero units.")
	}
	var (
		buf   bytes.Buffer
		units []provision.Unit
	)
	args := []string{"add-unit", app.GetName(), "--num-units", strconv.FormatUint(uint64(n), 10)}
	err := runCmd(false, &buf, &buf, args...)
	if err != nil {
		return nil, cmdError(buf.String(), err, args)
	}
	unitRe := regexp.MustCompile(fmt.Sprintf(
		`Unit '(%s/\d+)' added to service '%s'`, app.GetName(), app.GetName()),
	)
	scanner := bufio.NewScanner(&buf)
	scanner.Split(bufio.ScanLines)
	names := make([]string, n)
	units = make([]provision.Unit, n)
	i := 0
	for scanner.Scan() {
		matches := unitRe.FindStringSubmatch(scanner.Text())
		if len(matches) > 1 {
			units[i] = provision.Unit{Name: matches[1]}
			names[i] = matches[1]
			i++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, &provision.Error{Reason: buf.String(), Err: err}
	}
	if p.elbSupport() {
		p.enqueueUnits(app.GetName(), names...)
	}
	return units, nil
}

func (p *JujuProvisioner) removeUnit(app provision.App, unit provision.AppUnit) error {
	var (
		buf bytes.Buffer
		err error
	)
	cmd := []string{"remove-unit", unit.GetName()}
	// Sometimes juju gives the "no node" error. This is one of Zookeeper bad
	// behaviors. Let's try it multiple times before raising the error to the
	// user, and hope that someday we run away from Zookeeper.
	for i := 0; i < destroyTries; i++ {
		buf.Reset()
		err = runCmd(false, &buf, &buf, cmd...)
		if err != nil && unitNotFound(unit.GetName(), buf.Bytes()) {
			err = nil
		}
		if err == nil {
			break
		}
	}
	if err != nil {
		return cmdError(buf.String(), err, cmd)
	}
	if p.elbSupport() {
		router, err := Router()
		if err != nil {
			return err
		}
		err = router.RemoveRoute(app.GetName(), unit.GetInstanceId())
	}
	conn, collection := p.unitsCollection()
	defer conn.Close()
	collection.RemoveId(unit.GetName())
	go p.terminateMachines(app, unit)
	return err
}

func (p *JujuProvisioner) RemoveUnit(app provision.App, name string) error {
	var unit provision.AppUnit
	for _, unit = range app.ProvisionedUnits() {
		if unit.GetName() == name {
			break
		}
	}
	if unit.GetName() != name {
		return fmt.Errorf("App %q does not have a unit named %q.", app.GetName(), name)
	}
	return p.removeUnit(app, unit)
}

func (p *JujuProvisioner) InstallDeps(app provision.App, w io.Writer) error {
	return app.Run("/var/lib/tsuru/hooks/dependencies", w)
}

func (*JujuProvisioner) startedUnits(app provision.App) []provision.AppUnit {
	units := []provision.AppUnit{}
	allUnits := app.ProvisionedUnits()
	for _, unit := range allUnits {
		if status := unit.GetStatus(); status == provision.StatusStarted {
			units = append(units, unit)
		}
	}
	return units
}

func (p *JujuProvisioner) ExecuteCommandOnce(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	arguments := []string{"ssh", "-o", "StrictHostKeyChecking no", "-q"}
	units := app.ProvisionedUnits()
	length := len(units)
	for _, unit := range units {
		if length > 1 {
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
		log.Printf("[execute cmd] - running cmd %s on machine %s", cmd, strconv.Itoa(unit.GetMachine()))
		err := runCmd(true, stdout, stderr, cmdargs...)
		fmt.Fprintln(stdout)
		if err != nil {
			log.Printf("error on execute cmd %s on machine %s", cmd, strconv.Itoa(unit.GetMachine()))
			return err
		}
		return nil
	}
	return nil
}

func (p *JujuProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	arguments := []string{"ssh", "-o", "StrictHostKeyChecking no", "-q"}
	units := app.ProvisionedUnits()
	log.Printf("[execute cmd] - provisioned unit %#v", units)
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
		log.Printf("[execute cmd] - running cmd %s on machine %s", cmd, strconv.Itoa(unit.GetMachine()))
		err := runCmd(true, stdout, stderr, cmdargs...)
		fmt.Fprintln(stdout)
		if err != nil {
			log.Printf("error on execute cmd %s on machine %s", cmd, strconv.Itoa(unit.GetMachine()))
			return err
		}
	}
	return nil
}

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
	conn, collection := p.bootstrapCollection()
	defer conn.Close()
	_, err := collection.Upsert(nil, &m)
	return err
}

func (p *JujuProvisioner) bootstrapCollection() (*db.Storage, *mgo.Collection) {
	name, err := config.GetString("juju:bootstrap-collection")
	if err != nil {
		log.Fatalf("FATAL: %s.", err)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Fatalf("Failed to connect to the database: %s", err)
	}
	return conn, conn.Collection(name)
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

func (p *JujuProvisioner) heal(units []provision.Unit) {
	var inst instance
	conn, coll := p.unitsCollection()
	defer conn.Close()
	for _, unit := range units {
		err := coll.FindId(unit.Name).One(&inst)
		if err != nil {
			coll.Insert(instance{UnitName: unit.Name, InstanceID: unit.InstanceId})
		} else if unit.InstanceId == inst.InstanceID {
			continue
		} else {
			format := "[juju] instance-id of unit %q changed from %q to %q. Healing."
			log.Printf(format, unit.Name, inst.InstanceID, unit.InstanceId)
			if p.elbSupport() {
				router, err := Router()
				if err != nil {
					continue
				}
				router.RemoveRoute(unit.AppName, inst.InstanceID)
				err = router.AddRoute(unit.AppName, unit.InstanceId)
				if err != nil {
					format := "[juju] Could not register instance %q in the load balancer: %s."
					log.Printf(format, unit.InstanceId, err)
					continue
				}
			}
			if inst.InstanceID != "pending" {
				msg := queue.Message{
					Action: app.RegenerateApprcAndStart,
					Args:   []string{unit.AppName, unit.Name},
				}
				app.Enqueue(msg)
			}
			inst.InstanceID = unit.InstanceId
			coll.UpdateId(unit.Name, inst)
		}
	}
}

func (p *JujuProvisioner) CollectStatus() ([]provision.Unit, error) {
	units, err := p.collectStatus()
	if err != nil {
		return nil, err
	}
	go p.heal(units)
	return units, err
}

func (p *JujuProvisioner) Addr(app provision.App) (string, error) {
	if p.elbSupport() {
		router, err := Router()
		if err != nil {
			return "", err
		}
		return router.Addr(app.GetName())
	}
	units := app.ProvisionedUnits()
	if len(units) < 1 {
		return "", fmt.Errorf("App %q has no units.", app.GetName())
	}
	return units[0].GetIp(), nil
}

// instance represents a unit in the database.
type instance struct {
	UnitName   string `bson:"_id"`
	InstanceID string
}

type unit struct {
	AgentState    string `yaml:"agent-state"`
	Machine       int
	PublicAddress string `yaml:"public-address"`
}

type service struct {
	Units map[string]unit
	Charm string
}

type machine struct {
	AgentState    string `yaml:"agent-state"`
	IPAddress     string `yaml:"dns-name"`
	InstanceID    string `yaml:"instance-id"`
	InstanceState string `yaml:"instance-state"`
}

type jujuOutput struct {
	Services map[string]service
	Machines map[int]machine
}

func runCmd(filter bool, stdout, stderr io.Writer, args ...string) error {
	if filter {
		stdout = &Writer{stdout}
		stderr = &Writer{stderr}
	}
	return executor().Execute("juju", args, nil, stdout, stderr)
}

func cmdError(output string, err error, cmd []string) error {
	log.Printf("[juju] Failed to run cmd %q (%s):\n%s", strings.Join(cmd, " "), err, output)
	return &provision.Error{Reason: output, Err: err}
}

func execWithTimeout(timeout time.Duration, cmd string, args ...string) (output []byte, err error) {
	var buf safe.Buffer
	ch := make(chan []byte, 1)
	errCh := make(chan error, 1)
	command := osexec.Command(cmd, args...)
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

func unitNotFound(unitName string, output []byte) bool {
	re := regexp.MustCompile(fmt.Sprintf(`Service unit '%s' was not found$`, unitName))
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if re.Match(line) {
			return true
		}
	}
	return false
}

func unitStatus(instanceState, agentState, machineAgentState string) provision.Status {
	if instanceState == "error" ||
		machineAgentState == "start-error" ||
		strings.Contains(agentState, "error") {
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
