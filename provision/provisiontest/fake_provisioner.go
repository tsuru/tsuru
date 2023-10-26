// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provisiontest

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	uuid "github.com/nu7hatch/gouuid"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/routertest"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	logTypes "github.com/tsuru/tsuru/types/log"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
)

var (
	ProvisionerInstance *FakeProvisioner
	errNotProvisioned         = &provision.Error{Reason: "App is not provisioned."}
	uniqueIpCounter     int32 = 0

	_ provision.Provisioner           = &FakeProvisioner{}
	_ provision.InterAppProvisioner   = &FakeProvisioner{}
	_ provision.UpdatableProvisioner  = &FakeProvisioner{}
	_ provision.Provisioner           = &FakeProvisioner{}
	_ provision.LogsProvisioner       = &FakeProvisioner{}
	_ provision.MetricsProvisioner    = &FakeProvisioner{}
	_ provision.VolumeProvisioner     = &FakeProvisioner{}
	_ provision.AppFilterProvisioner  = &FakeProvisioner{}
	_ provision.ExecutableProvisioner = &FakeProvisioner{}
	_ provision.App                   = &FakeApp{}
	_ bind.App                        = &FakeApp{}
	_ provisionTypes.ResourceGetter   = &FakeApp{}
)

func init() {
	ProvisionerInstance = NewFakeProvisioner()
	provision.Register("fake", func() (provision.Provisioner, error) {
		return ProvisionerInstance, nil
	})
}

// Fake implementation for provision.App.
type FakeApp struct {
	name              string
	uuid              string
	cname             []string
	IP                string
	platform          string
	platformVersion   string
	units             []provision.Unit
	logs              []string
	logMut            sync.Mutex
	Commands          []string
	Memory            int64
	MilliCPU          int
	commMut           sync.Mutex
	Deploys           uint
	env               map[string]bindTypes.EnvVar
	serviceEnvs       []bindTypes.ServiceEnvVar
	serviceLock       sync.Mutex
	Pool              string
	UpdatePlatform    bool
	TeamOwner         string
	Teams             []string
	Tags              []string
	Metadata          appTypes.Metadata
	InternalAddresses []provision.AppInternalAddress
}

func NewFakeJob(name, pool, teamOwner string) *jobTypes.Job {
	job := &jobTypes.Job{
		Name:      name,
		Pool:      pool,
		TeamOwner: teamOwner,
		Teams:     []string{teamOwner},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "ubuntu:latest",
				Command:          []string{"echo", "hello world"},
			},
		},
	}
	return job
}

func NewFakeApp(name, platform string, units int) *FakeApp {
	return NewFakeAppWithPool(name, platform, "test-default", units)
}

func NewFakeAppWithPool(name, platform, pool string, units int) *FakeApp {
	repo, version := image.SplitImageName(platform)
	app := FakeApp{
		name:            name,
		platform:        repo,
		platformVersion: version,
		units:           make([]provision.Unit, units),
		Pool:            pool,
	}
	routertest.FakeRouter.AddBackend(context.TODO(), &app)
	namefmt := "%s-%d"
	for i := 0; i < units; i++ {
		val := atomic.AddInt32(&uniqueIpCounter, 1)
		app.units[i] = provision.Unit{
			ID:     fmt.Sprintf(namefmt, name, i),
			Status: provision.StatusStarted,
			IP:     fmt.Sprintf("10.10.10.%d", val),
			Address: &url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("10.10.10.%d:%d", val, val),
			},
		}
	}
	return &app
}

func (a *FakeApp) GetMilliCPU() int {
	return a.MilliCPU
}

func (a *FakeApp) GetCPUBurst() float64 {
	return 0
}

func (a *FakeApp) GetMemory() int64 {
	return a.Memory
}

func (a *FakeApp) GetTeamsName() []string {
	return a.Teams
}

func (a *FakeApp) GetCname() []string {
	return a.cname
}

func (a *FakeApp) GetServiceEnvs() []bindTypes.ServiceEnvVar {
	a.serviceLock.Lock()
	defer a.serviceLock.Unlock()
	return a.serviceEnvs
}

func (a *FakeApp) AddInstance(instanceArgs bind.AddInstanceArgs) error {
	a.serviceLock.Lock()
	defer a.serviceLock.Unlock()
	a.serviceEnvs = append(a.serviceEnvs, instanceArgs.Envs...)
	if instanceArgs.Writer != nil {
		instanceArgs.Writer.Write([]byte("add instance"))
	}
	return nil
}

func (a *FakeApp) RemoveInstance(instanceArgs bind.RemoveInstanceArgs) error {
	a.serviceLock.Lock()
	defer a.serviceLock.Unlock()
	lenBefore := len(a.serviceEnvs)
	for i := 0; i < len(a.serviceEnvs); i++ {
		se := a.serviceEnvs[i]
		if se.ServiceName == instanceArgs.ServiceName && se.InstanceName == instanceArgs.InstanceName {
			a.serviceEnvs = append(a.serviceEnvs[:i], a.serviceEnvs[i+1:]...)
			i--
		}
	}
	if len(a.serviceEnvs) == lenBefore {
		return errors.New("instance not found")
	}
	if instanceArgs.Writer != nil {
		instanceArgs.Writer.Write([]byte("remove instance"))
	}
	return nil
}

func (a *FakeApp) Logs() []string {
	a.logMut.Lock()
	defer a.logMut.Unlock()
	logs := make([]string, len(a.logs))
	copy(logs, a.logs)
	return logs
}

func (a *FakeApp) HasLog(source, unit, message string) bool {
	log := source + unit + message
	a.logMut.Lock()
	defer a.logMut.Unlock()
	for _, l := range a.logs {
		if l == log {
			return true
		}
	}
	return false
}

func (a *FakeApp) GetCommands() []string {
	a.commMut.Lock()
	defer a.commMut.Unlock()
	return a.Commands
}

func (a *FakeApp) Log(message, source, unit string) error {
	a.logMut.Lock()
	a.logs = append(a.logs, source+unit+message)
	a.logMut.Unlock()
	return nil
}

func (a *FakeApp) GetName() string {
	return a.name
}

func (a *FakeApp) GetUUID() (string, error) {
	if a.uuid != "" {
		return a.uuid, nil
	}
	uuidV4, err := uuid.NewV4()
	if err != nil {
		return "", errors.WithMessage(err, "failed to generate uuid v4")
	}
	a.uuid = uuidV4.String()
	return a.uuid, nil
}

func (a *FakeApp) GetPool() string {
	return a.Pool
}

func (a *FakeApp) GetPlatform() string {
	return a.platform
}

func (a *FakeApp) GetPlatformVersion() string {
	return a.platformVersion
}

func (a *FakeApp) GetDeploys() uint {
	return a.Deploys
}

func (a *FakeApp) GetTeamOwner() string {
	return a.TeamOwner
}

func (a *FakeApp) Units() ([]provision.Unit, error) {
	return a.units, nil
}

func (a *FakeApp) AddUnit(u provision.Unit) {
	a.units = append(a.units, u)
}

func (a *FakeApp) SetEnv(env bindTypes.EnvVar) {
	if a.env == nil {
		a.env = map[string]bindTypes.EnvVar{}
	}
	a.env[env.Name] = env
}

func (a *FakeApp) SetEnvs(setEnvs bind.SetEnvArgs) error {
	for _, env := range setEnvs.Envs {
		a.SetEnv(env)
	}
	return nil
}

func (a *FakeApp) UnsetEnvs(unsetEnvs bind.UnsetEnvArgs) error {
	for _, env := range unsetEnvs.VariableNames {
		delete(a.env, env)
	}
	return nil
}

func (a *FakeApp) Envs() map[string]bindTypes.EnvVar {
	return a.env
}

func (a *FakeApp) Run(cmd string, w io.Writer, args provision.RunArgs) error {
	a.commMut.Lock()
	a.Commands = append(a.Commands, fmt.Sprintf("ran %s", cmd))
	a.commMut.Unlock()
	return nil
}

func (a *FakeApp) GetUpdatePlatform() bool {
	return a.UpdatePlatform
}

func (app *FakeApp) GetRouters() []appTypes.AppRouter {
	return []appTypes.AppRouter{{Name: "fake"}}
}

func (app *FakeApp) GetAddresses() ([]string, error) {
	addr, err := routertest.FakeRouter.Addr(context.TODO(), app)
	if err != nil {
		return nil, err
	}
	return []string{addr}, nil
}

func (app *FakeApp) GetInternalBindableAddresses() ([]string, error) {
	var addresses []string
	for _, addr := range app.InternalAddresses {
		if addr.Version != "" {
			continue
		}
		addresses = append(addresses, fmt.Sprintf("%s://%s:%d", strings.ToLower(addr.Protocol), addr.Domain, addr.Port))
	}
	return addresses, nil
}

func (app *FakeApp) ListTags() []string {
	return app.Tags
}

func (app *FakeApp) GetMetadata() appTypes.Metadata {
	return app.Metadata
}

func (app *FakeApp) GetRegistry() (imgTypes.ImageRegistry, error) {
	return "", nil
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
	Name        string
	LogsEnabled bool
	outputs     chan []byte
	failures    chan failure
	apps        map[string]provisionedApp
	jobs        map[string]*provisionedJob
	mut         sync.RWMutex
	execs       map[string][]provision.ExecOptions
	execsMut    sync.Mutex
}

func NewFakeProvisioner() *FakeProvisioner {
	p := FakeProvisioner{Name: "fake"}
	p.outputs = make(chan []byte, 8)
	p.failures = make(chan failure, 8)
	p.apps = make(map[string]provisionedApp)
	p.jobs = make(map[string]*provisionedJob)
	p.execs = make(map[string][]provision.ExecOptions)
	return &p
}

func (p *FakeProvisioner) getError(method string) error {
	select {
	case fail := <-p.failures:
		if fail.method == method {
			return fail.err
		}
		p.failures <- fail
	default:
	}
	return nil
}

// Restarts returns the number of restarts for a given app.
func (p *FakeProvisioner) Restarts(a provision.App, process string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[a.GetName()].restarts[process]
}

// Starts returns the number of starts for a given app.
func (p *FakeProvisioner) Starts(app provision.App, process string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.GetName()].starts[process]
}

// Stops returns the number of stops for a given app.
func (p *FakeProvisioner) Stops(app provision.App, process string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.GetName()].stops[process]
}

func (p *FakeProvisioner) CustomData(app provision.App) map[string]interface{} {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.GetName()].lastData
}

// Execs return all exec calls to the given unit.
func (p *FakeProvisioner) Execs(unit string) []provision.ExecOptions {
	p.execsMut.Lock()
	defer p.execsMut.Unlock()
	return p.execs[unit]
}

// AllExecs return all exec calls to all units.
func (p *FakeProvisioner) AllExecs() map[string][]provision.ExecOptions {
	p.execsMut.Lock()
	defer p.execsMut.Unlock()
	all := map[string][]provision.ExecOptions{}
	for k, v := range p.execs {
		all[k] = v[:len(v):len(v)]
	}
	return all
}

// Provisioned checks whether the given app has been provisioned.
func (p *FakeProvisioner) Provisioned(app provision.App) bool {
	p.mut.RLock()
	defer p.mut.RUnlock()
	_, ok := p.apps[app.GetName()]
	return ok
}

// ProvisionedJob checks whether the given job has been provisioned.
func (p *FakeProvisioner) ProvisionedJob(jobName string) bool {
	p.mut.RLock()
	defer p.mut.RUnlock()
	_, ok := p.jobs[jobName]
	return ok
}

// JobExecutions returns the number of times a job has run
func (p *FakeProvisioner) JobExecutions(jobName string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	if j, ok := p.jobs[jobName]; ok {
		return j.executions
	}
	return 0
}

func (p *FakeProvisioner) GetUnits(app provision.App) []provision.Unit {
	p.mut.RLock()
	pApp := p.apps[app.GetName()]
	p.mut.RUnlock()
	return pApp.units
}

// GetAppFromUnitID returns an app from unitID
func (p *FakeProvisioner) GetAppFromUnitID(unitID string) (provision.App, error) {
	p.mut.RLock()
	defer p.mut.RUnlock()
	for _, a := range p.apps {
		for _, u := range a.units {
			if u.GetID() == unitID {
				return a.app, nil
			}
		}
	}
	return nil, errors.New("app not found")
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
// For instance, PrepareFailure("GitDeploy", errors.New("GitDeploy failed")) will
// cause next Deploy call to return the given error. Multiple calls to this
// method will enqueue failures, i.e. three calls to
// PrepareFailure("GitDeploy"...) means that the three next GitDeploy call will
// fail.
func (p *FakeProvisioner) PrepareFailure(method string, err error) {
	p.failures <- failure{method, err}
}

// Reset cleans up the FakeProvisioner, deleting all apps and their data. It
// also deletes prepared failures and output. It's like calling
// NewFakeProvisioner again, without all the allocations.
func (p *FakeProvisioner) Reset() {
	p.mut.Lock()
	p.apps = make(map[string]provisionedApp)
	p.mut.Unlock()

	p.mut.Lock()
	p.jobs = make(map[string]*provisionedJob)
	p.mut.Unlock()

	p.execsMut.Lock()
	p.execs = make(map[string][]provision.ExecOptions)
	p.execsMut.Unlock()

	uniqueIpCounter = 0

	for {
		select {
		case <-p.outputs:
		case <-p.failures:
		default:
			return
		}
	}
}

func (p *FakeProvisioner) Swap(ctx context.Context, app1, app2 provision.App, cnameOnly bool) error {
	return routertest.FakeRouter.Swap(ctx, app1, app2, cnameOnly)
}

func (p *FakeProvisioner) Deploy(ctx context.Context, args provision.DeployArgs) (string, error) {
	if err := p.getError("Deploy"); err != nil {
		return "", err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[args.App.GetName()]
	if !ok {
		return "", errNotProvisioned
	}
	if args.Version.VersionInfo().DeployImage != "" {
		pApp.image = args.Version.VersionInfo().DeployImage
	} else {
		pApp.image = args.Version.VersionInfo().BuildImage
	}
	args.Event.Write([]byte("Builder deploy called"))
	p.apps[args.App.GetName()] = pApp
	err := args.Version.CommitBaseImage()
	if err != nil {
		return "", err
	}
	err = args.Version.CommitSuccessful()
	if err != nil {
		return "", err
	}
	return args.Version.VersionInfo().DeployImage, nil
}

func (p *FakeProvisioner) Provision(ctx context.Context, app provision.App) error {
	if err := p.getError("Provision"); err != nil {
		return err
	}
	if p.Provisioned(app) {
		return &provision.Error{Reason: "App already provisioned."}
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	p.apps[app.GetName()] = provisionedApp{
		app:      app,
		restarts: make(map[string]int),
		starts:   make(map[string]int),
		stops:    make(map[string]int),
	}
	return nil
}

func (p *FakeProvisioner) Restart(ctx context.Context, app provision.App, process string, version appTypes.AppVersion, w io.Writer) error {
	if err := p.getError("Restart"); err != nil {
		return err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	pApp.restarts[process]++
	p.apps[app.GetName()] = pApp
	if w != nil {
		fmt.Fprintf(w, "restarting app")
	}
	return nil
}

func (p *FakeProvisioner) Start(ctx context.Context, app provision.App, process string, version appTypes.AppVersion, w io.Writer) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	pApp.starts[process]++
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) Destroy(ctx context.Context, app provision.App) error {
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

func (p *FakeProvisioner) DestroyVersion(ctx context.Context, app provision.App, version appTypes.AppVersion) error {
	if err := p.getError("DestroyVersion"); err != nil {
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

func (p *FakeProvisioner) AddUnits(ctx context.Context, app provision.App, n uint, process string, version appTypes.AppVersion, w io.Writer) error {
	_, err := p.AddUnitsToNode(app, n, process, w, "", version)
	return err
}

func (p *FakeProvisioner) AddUnitsToNode(app provision.App, n uint, process string, w io.Writer, nodeAddr string, version appTypes.AppVersion) ([]provision.Unit, error) {
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
	var addresses []*url.URL

	var versionNum int
	if version != nil {
		versionNum = version.Version()
	}
	for i := uint(0); i < n; i++ {
		val := atomic.AddInt32(&uniqueIpCounter, 1)
		var hostAddr string
		if nodeAddr != "" {
			hostAddr = net.URLToHost(nodeAddr)
		} else {
			hostAddr = fmt.Sprintf("10.10.10.%d", val)
		}
		unit := provision.Unit{
			ID:          fmt.Sprintf("%s-%d", name, pApp.unitLen),
			AppName:     name,
			Type:        platform,
			Status:      provision.StatusStarted,
			IP:          hostAddr,
			ProcessName: process,
			Address: &url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", hostAddr, val),
			},
			Version: versionNum,
		}
		addresses = append(addresses, unit.Address)
		pApp.units = append(pApp.units, unit)
		pApp.unitLen++
	}
	err := routertest.FakeRouter.AddRoutes(context.TODO(), app, addresses)
	if err != nil {
		return nil, err
	}
	result := make([]provision.Unit, int(n))
	copy(result, pApp.units[length:])
	p.apps[app.GetName()] = pApp
	if w != nil {
		fmt.Fprintf(w, "added %d units", n)
	}
	return result, nil
}

func (p *FakeProvisioner) RemoveUnits(ctx context.Context, app provision.App, n uint, process string, version appTypes.AppVersion, w io.Writer) error {
	if err := p.getError("RemoveUnits"); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("cannot remove 0 units")
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	var newUnits []provision.Unit
	removedCount := n
	var addresses []*url.URL
	for _, u := range pApp.units {
		if removedCount > 0 && u.ProcessName == process {
			removedCount--
			addresses = append(addresses, u.Address)
			continue
		}
		newUnits = append(newUnits, u)
	}
	err := routertest.FakeRouter.RemoveRoutes(ctx, app, addresses)
	if err != nil {
		return err
	}
	if removedCount > 0 {
		return errors.New("too many units to remove")
	}
	if w != nil {
		fmt.Fprintf(w, "removing %d units", n)
	}
	pApp.units = newUnits
	pApp.unitLen = len(newUnits)
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) AddUnit(app provision.App, unit provision.Unit) {
	p.mut.Lock()
	defer p.mut.Unlock()
	a := p.apps[app.GetName()]
	a.units = append(a.units, unit)
	a.unitLen++
	p.apps[app.GetName()] = a
}

func (p *FakeProvisioner) Units(ctx context.Context, apps ...provision.App) ([]provision.Unit, error) {
	if err := p.getError("Units"); err != nil {
		return nil, err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	var allUnits []provision.Unit
	for _, a := range apps {
		allUnits = append(allUnits, p.apps[a.GetName()].units...)
	}
	return allUnits, nil
}

func (p *FakeProvisioner) UnitsMetrics(ctx context.Context, a provision.App) ([]provision.UnitMetric, error) {
	if err := p.getError("UnitsMetrics"); err != nil {
		return nil, err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	var unitsMetrics []provision.UnitMetric
	for _, unit := range p.apps[a.GetName()].units {
		unitsMetrics = append(unitsMetrics, provision.UnitMetric{
			ID:     unit.ID,
			CPU:    "10m",
			Memory: "100Mi",
		})
	}
	return unitsMetrics, nil
}

func (p *FakeProvisioner) MockRoutableAddresses(app provision.App, addrs []appTypes.RoutableAddresses) {
	p.mut.Lock()
	defer p.mut.Unlock()
	a := p.apps[app.GetName()]
	a.mockAddrs = addrs
	p.apps[app.GetName()] = a
}

func (p *FakeProvisioner) RoutableAddresses(ctx context.Context, app provision.App) ([]appTypes.RoutableAddresses, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	a := p.apps[app.GetName()]
	if a.mockAddrs != nil {
		return a.mockAddrs, nil
	}
	units := a.units
	addrs := make([]*url.URL, len(units))
	for i := range units {
		addrs[i] = units[i].Address
	}
	return []appTypes.RoutableAddresses{{Addresses: addrs}}, nil
}

func (p *FakeProvisioner) SetUnitStatus(unit provision.Unit, status provision.Status) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	var units []provision.Unit
	if unit.AppName == "" {
		units = p.getAllUnits()
	} else {
		app, ok := p.apps[unit.AppName]
		if !ok {
			return errNotProvisioned
		}
		units = app.units
	}
	index := -1
	for i, unt := range units {
		if unt.ID == unit.ID {
			index = i
			unit.AppName = unt.AppName
			break
		}
	}
	if index < 0 {
		return &provision.UnitNotFoundError{ID: unit.ID}
	}
	app := p.apps[unit.AppName]
	app.units[index].Status = status
	p.apps[unit.AppName] = app
	return nil
}

func (p *FakeProvisioner) getAllUnits() []provision.Unit {
	var units []provision.Unit
	for _, app := range p.apps {
		units = append(units, app.units...)
	}
	return units
}

func (p *FakeProvisioner) Addr(app provision.App) (string, error) {
	if err := p.getError("Addr"); err != nil {
		return "", err
	}
	return routertest.FakeRouter.Addr(context.TODO(), app)
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
	pApp.cnames = append(pApp.cnames, cname)
	p.apps[app.GetName()] = pApp
	return routertest.FakeRouter.SetCName(context.TODO(), cname, app)
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
	pApp.cnames = []string{}
	p.apps[app.GetName()] = pApp
	return routertest.FakeRouter.UnsetCName(context.TODO(), cname, app)
}

func (p *FakeProvisioner) HasCName(app provision.App, cname string) bool {
	p.mut.RLock()
	pApp, ok := p.apps[app.GetName()]
	p.mut.RUnlock()
	for _, cnameApp := range pApp.cnames {
		if cnameApp == cname {
			return ok && true
		}
	}
	return false
}

func (p *FakeProvisioner) Stop(ctx context.Context, app provision.App, process string, version appTypes.AppVersion, w io.Writer) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	pApp.stops[process]++
	for i, u := range pApp.units {
		u.Status = provision.StatusStopped
		pApp.units[i] = u
	}
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) RegisterUnit(ctx context.Context, a provision.App, unitId string, customData map[string]interface{}) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	pa, ok := p.apps[a.GetName()]
	if !ok {
		return errors.New("app not found")
	}
	pa.lastData = customData
	for i, u := range pa.units {
		if u.ID == unitId {
			u.IP = u.IP + "-updated"
			pa.units[i] = u
			p.apps[a.GetName()] = pa
			return nil
		}
	}
	return &provision.UnitNotFoundError{ID: unitId}
}

func (p *FakeProvisioner) ExecuteCommand(ctx context.Context, opts provision.ExecOptions) error {
	p.execsMut.Lock()
	defer p.execsMut.Unlock()
	var err error
	units := opts.Units
	if len(units) == 0 {
		units = []string{"isolated"}
	}
	for _, unitID := range units {
		p.execs[unitID] = append(p.execs[unitID], opts)
		select {
		case output := <-p.outputs:
			select {
			case fail := <-p.failures:
				if fail.method == "ExecuteCommand" {
					opts.Stderr.Write(output)
					return fail.err
				}
				p.failures <- fail
			default:
				opts.Stdout.Write(output)
			}
		case fail := <-p.failures:
			if fail.method == "ExecuteCommand" {
				err = fail.err
				select {
				case output := <-p.outputs:
					opts.Stderr.Write(output)
				default:
				}
			} else {
				p.failures <- fail
			}
		case <-time.After(2e9):
			return errors.New("FakeProvisioner timed out waiting for output.")
		}
	}
	return err
}

func (p *FakeProvisioner) FilterAppsByUnitStatus(ctx context.Context, apps []provision.App, status []string) ([]provision.App, error) {
	filteredApps := []provision.App{}
	for i := range apps {
		units, _ := p.Units(ctx, apps[i])
		for _, u := range units {
			if stringInArray(u.Status.String(), status) {
				filteredApps = append(filteredApps, apps[i])
				break
			}
		}
	}
	return filteredApps, nil
}

func (p *FakeProvisioner) GetName() string {
	return p.Name
}

func (p *FakeProvisioner) DeleteVolume(ctx context.Context, volName, pool string) error {
	return nil
}

func (p *FakeProvisioner) ValidateVolume(ctx context.Context, vol *volumeTypes.Volume) error {
	return nil
}
func (p *FakeProvisioner) IsVolumeProvisioned(ctx context.Context, name, pool string) (bool, error) {
	return false, nil
}

func (p *FakeProvisioner) UpdateApp(ctx context.Context, old, new provision.App, w io.Writer) error {
	provApp := p.apps[old.GetName()]
	provApp.app = new
	if new.GetPool() != old.GetPool() {
		provApp.restarts[""]++
	}
	p.apps[old.GetName()] = provApp
	return nil
}

func (p *FakeProvisioner) InternalAddresses(ctx context.Context, a provision.App) ([]provision.AppInternalAddress, error) {
	return []provision.AppInternalAddress{
		{
			Domain:   fmt.Sprintf("%s-web.fake-cluster.local", a.GetName()),
			Port:     80,
			Protocol: "TCP",
			Process:  "web",
		},
		{
			Domain:   fmt.Sprintf("%s-logs.fake-cluster.local", a.GetName()),
			Port:     12201,
			Protocol: "UDP",
			Process:  "logs",
		},
		{
			Domain:   fmt.Sprintf("%s-logs-v2.fake-cluster.local", a.GetName()),
			Port:     12201,
			Protocol: "UDP",
			Process:  "logs",
			Version:  "2",
		},
		{
			Domain:   fmt.Sprintf("%s-web-v2.fake-cluster.local", a.GetName()),
			Port:     80,
			Protocol: "TCP",
			Process:  "web",
			Version:  "2",
		},
	}, nil

}

func (p *FakeProvisioner) ListLogs(ctx context.Context, obj logTypes.LogabbleObject, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	if !p.LogsEnabled {
		return nil, provision.ErrLogsUnavailable
	}
	return []appTypes.Applog{
		{
			Message: "Fake message from provisioner",
		},
	}, nil
}

func (p *FakeProvisioner) WatchLogs(ctx context.Context, obj logTypes.LogabbleObject, args appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
	if !p.LogsEnabled {
		return nil, provision.ErrLogsUnavailable
	}
	watcher := appTypes.NewMockLogWatcher()
	watcher.Enqueue(appTypes.Applog{Message: "Fake message from provisioner"})
	return watcher, nil
}

func stringInArray(value string, array []string) bool {
	for _, str := range array {
		if str == value {
			return true
		}
	}
	return false
}

type PipelineFakeProvisioner struct {
	*FakeProvisioner
	executedPipeline bool
}

func (p *PipelineFakeProvisioner) ExecutedPipeline() bool {
	return p.executedPipeline
}

func (p *PipelineFakeProvisioner) DeployPipeline() *action.Pipeline {
	act := action.Action{
		Name: "change-executed-pipeline",
		Forward: func(ctx action.FWContext) (action.Result, error) {
			p.executedPipeline = true
			return nil, nil
		},
		Backward: func(ctx action.BWContext) {
		},
	}
	actions := []*action.Action{&act}
	pipeline := action.NewPipeline(actions...)
	return pipeline
}

type PipelineErrorFakeProvisioner struct {
	*FakeProvisioner
}

func (p *PipelineErrorFakeProvisioner) DeployPipeline() *action.Pipeline {
	act := action.Action{
		Name: "error-pipeline",
		Forward: func(ctx action.FWContext) (action.Result, error) {
			return nil, errors.New("deploy error")
		},
		Backward: func(ctx action.BWContext) {
		},
	}
	actions := []*action.Action{&act}
	pipeline := action.NewPipeline(actions...)
	return pipeline
}

type provisionedApp struct {
	units     []provision.Unit
	app       provision.App
	restarts  map[string]int
	starts    map[string]int
	stops     map[string]int
	cnames    []string
	unitLen   int
	lastData  map[string]interface{}
	image     string
	mockAddrs []appTypes.RoutableAddresses
}

type provisionedJob struct {
	units      []provision.Unit
	job        *jobTypes.Job
	executions int
}

type AutoScaleProvisioner struct {
	*FakeProvisioner
	autoscales map[string][]provision.AutoScaleSpec
}

var _ provision.AutoScaleProvisioner = &AutoScaleProvisioner{}

func (p *AutoScaleProvisioner) GetAutoScale(ctx context.Context, app provision.App) ([]provision.AutoScaleSpec, error) {
	if p.autoscales == nil {
		return nil, nil
	}
	return p.autoscales[app.GetName()], nil
}

func (p *AutoScaleProvisioner) GetVerticalAutoScaleRecommendations(ctx context.Context, app provision.App) ([]provision.RecommendedResources, error) {
	if p.autoscales == nil {
		return nil, nil
	}
	return []provision.RecommendedResources{
		{Process: "p1", Recommendations: []provision.RecommendedProcessResources{{Type: "target", CPU: "100m", Memory: "100MiB"}}},
	}, nil
}

func (p *AutoScaleProvisioner) SetAutoScale(ctx context.Context, app provision.App, spec provision.AutoScaleSpec) error {
	if p.autoscales == nil {
		p.autoscales = make(map[string][]provision.AutoScaleSpec)
	}
	p.autoscales[app.GetName()] = append(p.autoscales[app.GetName()], spec)
	return nil
}

func (p *AutoScaleProvisioner) RemoveAutoScale(ctx context.Context, app provision.App, process string) error {
	if p.autoscales == nil {
		return nil
	}
	previous := p.autoscales[app.GetName()]
	p.autoscales[app.GetName()] = nil
	for _, spec := range previous {
		if spec.Process == process {
			continue
		}
		p.autoscales[app.GetName()] = append(p.autoscales[app.GetName()], spec)
	}
	return nil
}

type JobProvisioner struct {
	*FakeProvisioner
}

var _ provision.JobProvisioner = &JobProvisioner{}

// JobUnits returns information about units related to a specific Job or CronJob
func (p *JobProvisioner) JobUnits(ctx context.Context, job *jobTypes.Job) ([]provision.Unit, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	return p.jobs[job.Name].units, nil
}

// JobSchedule creates a cronjob object in the cluster
func (p *JobProvisioner) CreateJob(ctx context.Context, job *jobTypes.Job) (string, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	name := job.Name
	p.jobs[name] = &provisionedJob{
		units: []provision.Unit{},
		job:   job,
	}
	return name, nil
}

func (p *JobProvisioner) DestroyJob(ctx context.Context, job *jobTypes.Job) error {
	if !p.ProvisionedJob(job.Name) {
		return errNotProvisioned
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	delete(p.jobs, job.Name)
	return nil
}

func (p *JobProvisioner) UpdateJob(ctx context.Context, job *jobTypes.Job) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	j, ok := p.jobs[job.Name]
	if !ok {
		return errNotProvisioned
	}
	j.job = job
	p.jobs[job.Name] = j
	return nil
}

func (p *JobProvisioner) TriggerCron(ctx context.Context, name, pool string) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	j, ok := p.jobs[name]
	if !ok {
		return errNotProvisioned
	}
	j.executions++
	return nil
}

func (p *JobProvisioner) NewJobWithUnits(ctx context.Context, job *jobTypes.Job) (string, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	name := job.Name
	p.jobs[name] = &provisionedJob{
		units: []provision.Unit{
			{
				Name:        "unit1",
				ProcessName: "p1",
				Status:      "running",
			},
			{
				Name:        "unit2",
				ProcessName: "p2",
				Status:      "running",
			},
		},
		job: job,
	}
	return name, nil
}

func (*JobProvisioner) KillJobUnit(ctx context.Context, job *jobTypes.Job, unit string, force bool) error {
	if job.Name == "job1" && unit == "unit2" {
		return nil
	}
	return &provision.UnitNotFoundError{ID: unit}
}
