// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"errors"
	"fmt"
	"github.com/globocom/tsuru/provision"
	"io"
	"strconv"
	"sync"
	"time"
)

var errNotProvisioned = &provision.Error{Reason: "App is not provisioned."}

func init() {
	provision.Register("fake", &FakeProvisioner{})
}

// Fake implementation for provision.Unit.
type FakeUnit struct {
	Name       string
	Ip         string
	InstanceId string
	Machine    int
	Status     provision.Status
}

func (u *FakeUnit) GetName() string {
	return u.Name
}

func (u *FakeUnit) GetMachine() int {
	return u.Machine
}

func (u *FakeUnit) GetStatus() provision.Status {
	return u.Status
}

func (u *FakeUnit) GetInstanceId() string {
	return u.InstanceId
}

func (u *FakeUnit) GetIp() string {
	return u.Ip
}

// Fake implementation for provision.App.
type FakeApp struct {
	name     string
	platform string
	units    []provision.AppUnit
	logs     []string
	Commands []string
	logMut   sync.Mutex
	ready    bool
}

func NewFakeApp(name, platform string, units int) *FakeApp {
	app := FakeApp{
		name:     name,
		platform: platform,
		units:    make([]provision.AppUnit, units),
	}
	namefmt := "%s/%d"
	for i := 0; i < units; i++ {
		app.units[i] = &FakeUnit{
			Name:       fmt.Sprintf(namefmt, name, i),
			Machine:    i + 1,
			Status:     provision.StatusStarted,
			Ip:         fmt.Sprintf("10.10.10.%d", i+1),
			InstanceId: fmt.Sprintf("i-0%d", i+1),
		}
	}
	return &app
}

func (a *FakeApp) IsReady() bool {
	return a.ready
}

func (a *FakeApp) Ready() error {
	a.ready = true
	return nil
}

func (a *FakeApp) Log(message, source string) error {
	a.logMut.Lock()
	a.logs = append(a.logs, source+message)
	a.logMut.Unlock()
	return nil
}

func (a *FakeApp) GetName() string {
	return a.name
}

func (a *FakeApp) GetPlatform() string {
	return a.platform
}

func (a *FakeApp) ProvisionUnits() []provision.AppUnit {
	return a.units
}

func (a *FakeApp) AddUnit(u *FakeUnit) {
	a.units = append(a.units, u)
}

func (a *FakeApp) RemoveUnit(id string) error {
	index := -1
	for i, u := range a.units {
		if u.GetName() == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("Unit not found")
	}
	if index < len(a.units)-1 {
		a.units[index] = a.units[len(a.units)-1]
	}
	a.units = a.units[:len(a.units)-1]
	return nil
}

func (a *FakeApp) SetUnitStatus(s provision.Status, index int) {
	if index < len(a.units) {
		a.units[index].(*FakeUnit).Status = s
	}
}

func (a *FakeApp) Restart(w io.Writer) error {
	a.Commands = append(a.Commands, "restart")
	return nil
}

func (a *FakeApp) Run(cmd string, w io.Writer) error {
	a.Commands = append(a.Commands, fmt.Sprintf("ran %s", cmd))
	return nil
}

type Cmd struct {
	Cmd  string
	Args []string
	App  provision.App
}

type failure struct {
	method string
	err    error
}

// Fake implementation for provision.Provisioner.
type FakeProvisioner struct {
	cmds     []Cmd
	cmdMut   sync.Mutex
	outputs  chan []byte
	failures chan failure
	apps     map[string]provisionedApp
	mut      sync.RWMutex
}

func NewFakeProvisioner() *FakeProvisioner {
	p := FakeProvisioner{}
	p.outputs = make(chan []byte, 8)
	p.failures = make(chan failure, 8)
	p.apps = make(map[string]provisionedApp)
	return &p
}

func (p *FakeProvisioner) getError(method string) error {
	select {
	case fail := <-p.failures:
		if fail.method == method {
			return fail.err
		}
		p.failures <- fail
	case <-time.After(1e6):
	}
	return nil
}

// Restarts returns the number of restarts for a given app.
func (p *FakeProvisioner) Restarts(app provision.App) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.GetName()].restarts
}

// InstalledDeps returns the number of InstallDeps calls for the given app.
func (p *FakeProvisioner) InstalledDeps(app provision.App) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.GetName()].installDeps
}

// Returns the number of calls to restart.
// GetCmds returns a list of commands executed in an app. If you don't specify
// the command (an empty string), it will return all commands executed in the
// given app.
func (p *FakeProvisioner) GetCmds(cmd string, app provision.App) []Cmd {
	var cmds []Cmd
	p.cmdMut.Lock()
	for _, c := range p.cmds {
		if (cmd == "" || c.Cmd == cmd) && app.GetName() == c.App.GetName() {
			cmds = append(cmds, c)
		}
	}
	p.cmdMut.Unlock()
	return cmds
}

// Provisioned checks whether the given app has been provisioned.
func (p *FakeProvisioner) Provisioned(app provision.App) bool {
	p.mut.RLock()
	defer p.mut.RUnlock()
	_, ok := p.apps[app.GetName()]
	return ok
}

func (p *FakeProvisioner) GetUnits(app provision.App) []provision.Unit {
	p.mut.RLock()
	pApp, _ := p.apps[app.GetName()]
	p.mut.RUnlock()
	return pApp.units
}

// PrepareOutput sends the given slice of bytes to a queue of outputs.
//
// Each prepared output will be used in the ExecuteCommand. It might be sent to
// the standard output or standard error. See ExecuteCommand docs for more
// details.
func (p *FakeProvisioner) PrepareOutput(b []byte) {
	p.outputs <- b
}

// PrepareFailure prepares a failure for the given method name.
//
// For instance, PrepareFailure("Deploy", errors.New("Deploy failed")) will
// cause next Deploy call to return the given error. Multiple calls to this
// method will enqueue failures, i.e. three calls to
// PrepareFailure("Deploy"...) means that the three next Deploy call will fail.
func (p *FakeProvisioner) PrepareFailure(method string, err error) {
	p.failures <- failure{method, err}
}

// Reset cleans up the FakeProvisioner, deleting all apps and their data. It
// also deletes prepared failures and output. It's like calling
// NewFakeProvisioner again, without all the allocations.
func (p *FakeProvisioner) Reset() {
	p.cmdMut.Lock()
	p.cmds = nil
	p.cmdMut.Unlock()

	p.mut.Lock()
	p.apps = make(map[string]provisionedApp)
	p.mut.Unlock()

	for {
		select {
		case <-p.outputs:
		case <-p.failures:
		default:
			return
		}
	}
}

func (p *FakeProvisioner) Deploy(app provision.App, version string, w io.Writer) error {
	if err := p.getError("Deploy"); err != nil {
		return err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	w.Write([]byte("Deploy called"))
	pApp.version = version
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) Provision(app provision.App) error {
	if err := p.getError("Provision"); err != nil {
		return err
	}
	if p.Provisioned(app) {
		return &provision.Error{Reason: "App already provisioned."}
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	p.apps[app.GetName()] = provisionedApp{
		app:     app,
		unitLen: 1,
		units: []provision.Unit{
			{
				Name:       app.GetName() + "/0",
				AppName:    app.GetName(),
				Type:       app.GetPlatform(),
				Status:     provision.StatusStarted,
				InstanceId: "i-080",
				Ip:         "10.10.10.1",
				Machine:    1,
			},
		},
	}
	return nil
}

func (p *FakeProvisioner) Restart(app provision.App) error {
	if err := p.getError("Restart"); err != nil {
		return err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	pApp.restarts++
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) Destroy(app provision.App) error {
	if err := p.getError("Destroy"); err != nil {
		return err
	}
	if !p.Provisioned(app) {
		return errNotProvisioned
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	delete(p.apps, app.GetName())
	return nil
}

func (p *FakeProvisioner) AddUnits(app provision.App, n uint) ([]provision.Unit, error) {
	if err := p.getError("AddUnits"); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("Cannot add 0 units.")
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return nil, errNotProvisioned
	}
	name := app.GetName()
	platform := app.GetPlatform()
	length := uint(len(pApp.units))
	for i := uint(0); i < n; i++ {
		unit := provision.Unit{
			Name:       fmt.Sprintf("%s/%d", name, pApp.unitLen),
			AppName:    name,
			Type:       platform,
			Status:     provision.StatusStarted,
			InstanceId: fmt.Sprintf("i-08%d", length+i),
			Ip:         fmt.Sprintf("10.10.10.%d", length+i),
			Machine:    int(length + i),
		}
		pApp.units = append(pApp.units, unit)
		pApp.unitLen++
	}
	result := make([]provision.Unit, int(n))
	copy(result, pApp.units[length:])
	p.apps[app.GetName()] = pApp
	return result, nil
}

func (p *FakeProvisioner) RemoveUnit(app provision.App, name string) error {
	if err := p.getError("RemoveUnit"); err != nil {
		return err
	}
	index := -1
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	for i, unit := range pApp.units {
		if unit.Name == name {
			index = i
			break
		}
	}
	if index == -1 {
		return errors.New("Unit not found.")
	}
	copy(pApp.units[index:], pApp.units[index+1:])
	pApp.units = pApp.units[:len(pApp.units)-1]
	pApp.unitLen--
	p.apps[app.GetName()] = pApp
	return nil
}

// ExecuteCommand will pretend to execute the given command, recording data
// about it.
//
// The output of the command must be prepared with PrepareOutput, and failures
// must be prepared with PrepareFailure. In case of failure, the prepared
// output will be sent to the standard error stream, otherwise, it will be sent
// to the standard error stream.
//
// When there is no output nor failure prepared, ExecuteCommand will return a
// timeout error.
func (p *FakeProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	var (
		output []byte
		err    error
	)
	command := Cmd{
		Cmd:  cmd,
		Args: args,
		App:  app,
	}
	p.cmdMut.Lock()
	p.cmds = append(p.cmds, command)
	p.cmdMut.Unlock()
	select {
	case output = <-p.outputs:
		select {
		case fail := <-p.failures:
			if fail.method == "ExecuteCommand" {
				stderr.Write(output)
				return fail.err
			}
			p.failures <- fail
		case <-time.After(1e6):
			stdout.Write(output)
		}
	case fail := <-p.failures:
		if fail.method == "ExecuteCommand" {
			err = fail.err
			select {
			case output = <-p.outputs:
				stderr.Write(output)
			case <-time.After(1e6):
			}
		} else {
			p.failures <- fail
		}
	case <-time.After(2e9):
		return errors.New("FakeProvisioner timed out waiting for output.")
	}
	return err
}

func (p *FakeProvisioner) CollectStatus() ([]provision.Unit, error) {
	if err := p.getError("CollectStatus"); err != nil {
		return nil, err
	}
	units := make([]provision.Unit, len(p.apps))
	i := 0
	p.mut.RLock()
	defer p.mut.RUnlock()
	for name, a := range p.apps {
		unit := provision.Unit{
			Name:       name + "/0",
			AppName:    name,
			Type:       a.app.GetPlatform(),
			Status:     "started",
			InstanceId: fmt.Sprintf("i-0%d", 800+i+1),
			Ip:         "10.10.10." + strconv.Itoa(i+1),
			Machine:    i + 1,
		}
		units[i] = unit
		i++
	}
	return units, nil
}

func (p *FakeProvisioner) Addr(app provision.App) (string, error) {
	if err := p.getError("Addr"); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.fake-lb.tsuru.io", app.GetName()), nil
}

func (p *FakeProvisioner) InstallDeps(app provision.App, w io.Writer) error {
	if err := p.getError("InstallDeps"); err != nil {
		return err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	pApp.installDeps++
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) SetCName(app provision.App, cname string) error {
	if err := p.getError("SetCName"); err != nil {
		return err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	pApp.cname = cname
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) UnsetCName(app provision.App, cname string) error {
	if err := p.getError("UnsetCName"); err != nil {
		return err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	pApp.cname = ""
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) HasCName(app provision.App, cname string) bool {
	p.mut.RLock()
	pApp, ok := p.apps[app.GetName()]
	p.mut.RUnlock()
	return ok && pApp.cname == cname
}

type provisionedApp struct {
	units       []provision.Unit
	app         provision.App
	restarts    int
	installDeps int
	version     string
	cname       string
	unitLen     int
}
