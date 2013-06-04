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
	apps        []provision.App
	units       map[string][]provision.Unit
	unitLen     uint
	cmds        []Cmd
	outputs     chan []byte
	failures    chan failure
	cmdMut      sync.Mutex
	unitMut     sync.Mutex
	restarts    map[string]int
	restMut     sync.Mutex
	installDeps map[string]int
	depsMut     sync.Mutex
	versions    map[string]string
	cnames      map[string]string
}

func NewFakeProvisioner() *FakeProvisioner {
	p := FakeProvisioner{}
	p.outputs = make(chan []byte, 8)
	p.failures = make(chan failure, 8)
	p.units = make(map[string][]provision.Unit)
	p.restarts = make(map[string]int)
	p.installDeps = make(map[string]int)
	p.cnames = make(map[string]string)
	p.versions = make(map[string]string)
	p.unitLen = 0
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

func (p *FakeProvisioner) Restarts(app provision.App) int {
	p.restMut.Lock()
	defer p.restMut.Unlock()
	return p.restarts[app.GetName()]
}

func (p *FakeProvisioner) InstalledDeps(app provision.App) int {
	p.depsMut.Lock()
	defer p.depsMut.Unlock()
	return p.installDeps[app.GetName()]
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

func (p *FakeProvisioner) FindApp(app provision.App) int {
	for i, a := range p.apps {
		if a.GetName() == app.GetName() {
			return i
		}
	}
	return -1
}

func (p *FakeProvisioner) GetUnits(app provision.App) []provision.Unit {
	p.unitMut.Lock()
	defer p.unitMut.Unlock()
	return p.units[app.GetName()]
}

func (p *FakeProvisioner) PrepareOutput(b []byte) {
	p.outputs <- b
}

func (p *FakeProvisioner) PrepareFailure(method string, err error) {
	p.failures <- failure{method, err}
}

func (p *FakeProvisioner) Reset() {
	p.unitMut.Lock()
	p.units = make(map[string][]provision.Unit)
	p.unitMut.Unlock()

	p.cmdMut.Lock()
	p.cmds = nil
	p.cmdMut.Unlock()

	p.restMut.Lock()
	p.restarts = make(map[string]int)
	p.restMut.Unlock()

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
	index := p.FindApp(app)
	if index < 0 {
		return &provision.Error{Reason: "App is not provisioned."}
	}
	w.Write([]byte("Deploy called"))
	p.versions[app.GetName()] = version
	return nil
}

func (p *FakeProvisioner) Provision(app provision.App) error {
	if err := p.getError("Provision"); err != nil {
		return err
	}
	index := p.FindApp(app)
	if index > -1 {
		return &provision.Error{Reason: "App already provisioned."}
	}
	p.apps = append(p.apps, app)
	p.unitMut.Lock()
	p.units[app.GetName()] = []provision.Unit{
		{
			Name:       app.GetName() + "/0",
			AppName:    app.GetName(),
			Type:       app.GetPlatform(),
			Status:     provision.StatusStarted,
			InstanceId: "i-080",
			Ip:         "10.10.10.1",
			Machine:    1,
		},
	}
	p.unitLen++
	p.unitMut.Unlock()
	return nil
}

func (p *FakeProvisioner) Restart(app provision.App) error {
	if err := p.getError("Restart"); err != nil {
		return err
	}
	p.restMut.Lock()
	v := p.restarts[app.GetName()]
	v++
	p.restarts[app.GetName()] = v
	p.restMut.Unlock()
	return nil
}

func (p *FakeProvisioner) Destroy(app provision.App) error {
	if err := p.getError("Destroy"); err != nil {
		return err
	}
	index := p.FindApp(app)
	if index == -1 {
		return &provision.Error{Reason: "App is not provisioned."}
	}
	copy(p.apps[index:], p.apps[index+1:])
	p.apps = p.apps[:len(p.apps)-1]
	p.unitMut.Lock()
	delete(p.units, app.GetName())
	p.unitLen = 0
	p.unitMut.Unlock()
	p.restMut.Lock()
	delete(p.restarts, app.GetName())
	p.restMut.Unlock()
	return nil
}

func (p *FakeProvisioner) AddUnits(app provision.App, n uint) ([]provision.Unit, error) {
	if err := p.getError("AddUnits"); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("Cannot add 0 units.")
	}
	index := p.FindApp(app)
	if index < 0 {
		return nil, errors.New("App is not provisioned.")
	}
	name := app.GetName()
	platform := app.GetPlatform()
	p.unitMut.Lock()
	defer p.unitMut.Unlock()
	length := uint(len(p.units[name]))
	for i := uint(0); i < n; i++ {
		unit := provision.Unit{
			Name:       fmt.Sprintf("%s/%d", name, p.unitLen),
			AppName:    name,
			Type:       platform,
			Status:     provision.StatusStarted,
			InstanceId: fmt.Sprintf("i-08%d", length+i),
			Ip:         fmt.Sprintf("10.10.10.%d", length+i),
			Machine:    int(length + i),
		}
		p.units[name] = append(p.units[name], unit)
		p.unitLen++
	}
	result := make([]provision.Unit, int(n))
	copy(result, p.units[name][length:])
	return result, nil
}

func (p *FakeProvisioner) RemoveUnit(app provision.App, name string) error {
	if err := p.getError("RemoveUnit"); err != nil {
		return err
	}
	index := -1
	appName := app.GetName()
	if index := p.FindApp(app); index < 0 {
		return errors.New("App is not provisioned.")
	}
	p.unitMut.Lock()
	defer p.unitMut.Unlock()
	for i, unit := range p.units[appName] {
		if unit.Name == name {
			index = i
			break
		}
	}
	if index == -1 {
		return errors.New("Unit not found.")
	}
	copy(p.units[appName][index:], p.units[appName][index+1:])
	p.units[appName] = p.units[appName][:len(p.units[appName])-1]
	p.unitLen--
	return nil
}

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
	for i, app := range p.apps {
		unit := provision.Unit{
			Name:       app.GetName() + "/0",
			AppName:    app.GetName(),
			Type:       app.GetPlatform(),
			Status:     "started",
			InstanceId: fmt.Sprintf("i-0%d", 800+i+1),
			Ip:         "10.10.10." + strconv.Itoa(i+1),
			Machine:    i + 1,
		}
		units[i] = unit
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
	p.depsMut.Lock()
	v := p.installDeps[app.GetName()]
	v++
	p.installDeps[app.GetName()] = v
	p.depsMut.Unlock()
	return nil
}

func (p *FakeProvisioner) SetCName(app provision.App, cname string) error {
	p.cnames[app.GetName()] = cname
	return nil
}

func (p *FakeProvisioner) UnsetCName(app provision.App, cname string) error {
	delete(p.cnames, app.GetName())
	return nil
}

func (p *FakeProvisioner) HasCName(app provision.App, cname string) bool {
	got, ok := p.cnames[app.GetName()]
	if !ok {
		return false
	}
	return got == cname
}
