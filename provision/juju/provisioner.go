// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/db/storage"
	"github.com/globocom/tsuru/deploy"
	"github.com/globocom/tsuru/exec"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/router"
	_ "github.com/globocom/tsuru/router/elb"
	"github.com/globocom/tsuru/safe"
	"github.com/tsuru/config"
	"io"
	"labix.org/v2/mgo/bson"
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

func (p *JujuProvisioner) unitsCollection() *storage.Collection {
	name, err := config.GetString("juju:units-collection")
	if err != nil {
		log.Fatalf("FATAL: %s.", err)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Fatalf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
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

func (p *JujuProvisioner) Start(app provision.App) error {
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "/var/lib/tsuru/hooks/start")
	if err != nil {
		msg := fmt.Sprintf("Failed to start the app (%s): %s", err, buf.String())
		app.Log(msg, "tsuru-provisioner")
		return &provision.Error{Reason: buf.String(), Err: err}
	}
	return nil
}

func (p *JujuProvisioner) Stop(app provision.App) error {
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "/var/lib/tsuru/hooks/stop")
	if err != nil {
		msg := fmt.Sprintf("Failed to stop the app (%s): %s", err, buf.String())
		app.Log(msg, "tsuru-provisioner")
		return &provision.Error{Reason: buf.String(), Err: err}
	}
	return nil
}

func (JujuProvisioner) Swap(app1, app2 provision.App) error {
	r, err := Router()
	if err != nil {
		return err
	}
	return r.Swap(app1.GetName(), app2.GetName())
}

func (p *JujuProvisioner) Deploy(a provision.App, version string, w io.Writer) error {
	var buf bytes.Buffer
	setOption := []string{"set", a.GetName(), "app-version=" + version}
	if err := runCmd(true, &buf, &buf, setOption...); err != nil {
		log.Errorf("juju: Failed to set app-version. Error: %s.\nCommand output: %s", err, &buf)
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
			log.Errorf("Failed to destroy unit %q from the app %q: %s", u.GetName(), app.GetName(), out)
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
	collection := p.unitsCollection()
	defer collection.Close()
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
	collection := p.unitsCollection()
	defer collection.Close()
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
	return app.Run("/var/lib/tsuru/hooks/dependencies", w, false)
}

func (*JujuProvisioner) startedUnits(app provision.App) []provision.AppUnit {
	units := []provision.AppUnit{}
	allUnits := app.ProvisionedUnits()
	for _, unit := range allUnits {
		if unit.Available() {
			units = append(units, unit)
		}
	}
	return units
}

func (*JujuProvisioner) executeCommandViaSSH(stdout, stderr io.Writer, machine int, cmd string, args ...string) error {
	arguments := []string{"ssh", "-o", "StrictHostKeyChecking no", "-q"}
	arguments = append(arguments, strconv.Itoa(machine), cmd)
	arguments = append(arguments, args...)
	err := runCmd(true, stdout, stderr, arguments...)
	fmt.Fprintln(stdout)
	if err != nil {
		log.Errorf("error on execute cmd %s on machine %d", cmd, machine)
		return err
	}
	return nil
}

func (p *JujuProvisioner) ExecuteCommandOnce(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	units := p.startedUnits(app)
	if len(units) > 0 {
		unit := units[0]
		return p.executeCommandViaSSH(stdout, stderr, unit.GetMachine(), cmd, args...)
	}
	return nil
}

func (p *JujuProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	units := p.startedUnits(app)
	log.Debugf("[execute cmd] - provisioned unit %#v", units)
	length := len(units)
	for i, unit := range units {
		if length > 1 {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stdout, "Output from unit %q:\n\n", unit.GetName())
		}
		err := p.executeCommandViaSSH(stdout, stderr, unit.GetMachine(), cmd, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *JujuProvisioner) heal(units []provision.Unit) {
	var inst instance
	coll := p.unitsCollection()
	defer coll.Close()
	for _, unit := range units {
		err := coll.FindId(unit.Name).One(&inst)
		if err != nil {
			coll.Insert(instance{UnitName: unit.Name, InstanceID: unit.InstanceId})
		} else if unit.InstanceId == inst.InstanceID {
			continue
		} else {
			format := "[juju] instance-id of unit %q changed from %q to %q. Healing."
			log.Debugf(format, unit.Name, inst.InstanceID, unit.InstanceId)
			if p.elbSupport() {
				router, err := Router()
				if err != nil {
					continue
				}
				router.RemoveRoute(unit.AppName, inst.InstanceID)
				err = router.AddRoute(unit.AppName, unit.InstanceId)
				if err != nil {
					format := "[juju] Could not register instance %q in the load balancer: %s."
					log.Errorf(format, unit.InstanceId, err)
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

func (p *JujuProvisioner) Addr(app provision.App) (string, error) {
	if p.elbSupport() {
		router, err := Router()
		if err != nil {
			return "", err
		}
		addr, err := router.Addr(app.GetName())
		if err != nil {
			return "", fmt.Errorf("There is no ACTIVE Load Balancer named %s", app.GetName())
		}
		return addr, nil
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
	log.Errorf("[juju] Failed to run cmd %q (%s):\n%s", strings.Join(cmd, " "), err, output)
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

func (p *JujuProvisioner) DeployPipeline() *action.Pipeline {
	return nil
}
