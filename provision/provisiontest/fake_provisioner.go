// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provisiontest

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	logTypes "github.com/tsuru/tsuru/types/log"
	provTypes "github.com/tsuru/tsuru/types/provision"
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
)

func init() {
	ProvisionerInstance = NewFakeProvisioner()
	provision.Register("fake", func() (provision.Provisioner, error) {
		return ProvisionerInstance, nil
	})
}

func NewFakeJob(name, pool, teamOwner string) *jobTypes.Job {
	job := &jobTypes.Job{
		Name:      name,
		Pool:      pool,
		TeamOwner: teamOwner,
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

func NewFakeApp(name, platform string, units int) *appTypes.App {
	return NewFakeAppWithPool(name, platform, "test-default", units)
}

func NewFakeAppWithPool(name, platform, pool string, units int) *appTypes.App {
	repo, version := image.SplitImageName(platform)
	app := appTypes.App{
		Name:            name,
		Platform:        repo,
		PlatformVersion: version,
		Pool:            pool,
	}
	routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	//namefmt := "%s-%d"
	/*
		for i := 0; i < units; i++ {
			val := atomic.AddInt32(&uniqueIpCounter, 1)
			app.units[i] = provTypes.Unit{
				ID:     fmt.Sprintf(namefmt, name, i),
				Status: provTypes.UnitStatusStarted,
				IP:     fmt.Sprintf("10.10.10.%d", val),
				Address: &url.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("10.10.10.%d:%d", val, val),
				},
			}
		}*/
	return &app
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
func (p *FakeProvisioner) Restarts(a *appTypes.App, process string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[a.Name].restarts[process]
}

// Restarts returns the number of restarts for a given app.
func (p *FakeProvisioner) RestartsByVersion(a *appTypes.App, version string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[a.Name].restartsByVersion[version]
}

// Starts returns the number of starts for a given app.
func (p *FakeProvisioner) Starts(app *appTypes.App, process string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.Name].starts[process]
}

// Stops returns the number of stops for a given app.
func (p *FakeProvisioner) Stops(app *appTypes.App, process string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.Name].stops[process]
}

func (p *FakeProvisioner) CustomData(app *appTypes.App) map[string]interface{} {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.Name].lastData
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
func (p *FakeProvisioner) Provisioned(app *appTypes.App) bool {
	p.mut.RLock()
	defer p.mut.RUnlock()
	_, ok := p.apps[app.Name]
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

func (p *FakeProvisioner) GetUnits(app *appTypes.App) []provTypes.Unit {
	p.mut.RLock()
	pApp := p.apps[app.Name]
	p.mut.RUnlock()
	return pApp.units
}

// GetAppFromUnitID returns an app from unitID
func (p *FakeProvisioner) GetAppFromUnitID(unitID string) (*appTypes.App, error) {
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

func (p *FakeProvisioner) Deploy(ctx context.Context, args provision.DeployArgs) (string, error) {
	if err := p.getError("Deploy"); err != nil {
		return "", err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[args.App.Name]
	if !ok {
		return "", errNotProvisioned
	}
	if args.Version.VersionInfo().DeployImage != "" {
		pApp.image = args.Version.VersionInfo().DeployImage
	} else {
		pApp.image = args.Version.VersionInfo().BuildImage
	}
	args.Event.Write([]byte("Builder deploy called"))
	p.apps[args.App.Name] = pApp
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

func (p *FakeProvisioner) Provision(ctx context.Context, app *appTypes.App) error {
	if err := p.getError("Provision"); err != nil {
		return err
	}
	if p.Provisioned(app) {
		return &provision.Error{Reason: "App already provisioned."}
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	p.apps[app.Name] = provisionedApp{
		app:      app,
		restarts: make(map[string]int),
		starts:   make(map[string]int),
		stops:    make(map[string]int),

		restartsByVersion: make(map[string]int),
	}
	return nil
}

func (p *FakeProvisioner) Restart(ctx context.Context, app *appTypes.App, process string, version appTypes.AppVersion, w io.Writer) error {
	if err := p.getError("Restart"); err != nil {
		return err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.Name]
	if !ok {
		return errNotProvisioned
	}
	pApp.restarts[process]++

	if version == nil {
		pApp.restartsByVersion[""]++
	} else {
		key := strconv.Itoa(version.Version())
		pApp.restartsByVersion[key]++
	}

	p.apps[app.Name] = pApp
	if w != nil {
		fmt.Fprintf(w, "restarting app")
	}
	return nil
}

func (p *FakeProvisioner) Start(ctx context.Context, app *appTypes.App, process string, version appTypes.AppVersion, w io.Writer) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.Name]
	if !ok {
		return errNotProvisioned
	}
	pApp.starts[process]++
	p.apps[app.Name] = pApp
	return nil
}

func (p *FakeProvisioner) Destroy(ctx context.Context, app *appTypes.App) error {
	if err := p.getError("Destroy"); err != nil {
		return err
	}
	if !p.Provisioned(app) {
		return errNotProvisioned
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	delete(p.apps, app.Name)
	return nil
}

func (p *FakeProvisioner) DestroyVersion(ctx context.Context, app *appTypes.App, version appTypes.AppVersion) error {
	if err := p.getError("DestroyVersion"); err != nil {
		return err
	}
	if !p.Provisioned(app) {
		return errNotProvisioned
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	delete(p.apps, app.Name)
	return nil
}

func (p *FakeProvisioner) AddUnits(ctx context.Context, app *appTypes.App, n uint, process string, version appTypes.AppVersion, w io.Writer) error {
	_, err := p.AddUnitsToNode(app, n, process, w, "", version)
	return err
}

func (p *FakeProvisioner) AddUnitsToNode(app *appTypes.App, n uint, process string, w io.Writer, nodeAddr string, version appTypes.AppVersion) ([]provTypes.Unit, error) {
	if err := p.getError("AddUnits"); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("Cannot add 0 units.")
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.Name]
	if !ok {
		return nil, errNotProvisioned
	}
	name := app.Name
	platform := app.Platform
	length := uint(len(pApp.units))

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
		unit := provTypes.Unit{
			ID:          fmt.Sprintf("%s-%d", name, pApp.unitLen),
			AppName:     name,
			Type:        platform,
			Status:      provTypes.UnitStatusStarted,
			IP:          hostAddr,
			ProcessName: process,
			Address: &url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", hostAddr, val),
			},
			Version: versionNum,
		}
		pApp.units = append(pApp.units, unit)
		pApp.unitLen++
	}

	result := make([]provTypes.Unit, int(n))

	copy(result, pApp.units[length:])
	p.apps[app.Name] = pApp
	if w != nil {
		fmt.Fprintf(w, "added %d units", n)
	}
	return result, nil
}

func (p *FakeProvisioner) RemoveUnits(ctx context.Context, app *appTypes.App, n uint, process string, version appTypes.AppVersion, w io.Writer) error {
	if err := p.getError("RemoveUnits"); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("cannot remove 0 units")
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.Name]
	if !ok {
		return errNotProvisioned
	}
	var newUnits []provTypes.Unit
	removedCount := n
	for _, u := range pApp.units {
		if removedCount > 0 && u.ProcessName == process {
			removedCount--
			continue
		}
		newUnits = append(newUnits, u)
	}
	err := routertest.FakeRouter.RemoveBackend(ctx, app)
	if err != router.ErrBackendNotFound && err != nil {
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
	p.apps[app.Name] = pApp
	return nil
}

func (p *FakeProvisioner) AddUnit(app *appTypes.App, unit provTypes.Unit) {
	p.mut.Lock()
	defer p.mut.Unlock()
	a := p.apps[app.Name]
	a.units = append(a.units, unit)
	a.unitLen++
	p.apps[app.Name] = a
}

func (p *FakeProvisioner) Units(ctx context.Context, apps ...*appTypes.App) ([]provTypes.Unit, error) {
	if err := p.getError("Units"); err != nil {
		return nil, err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	var allUnits []provTypes.Unit
	for _, a := range apps {
		allUnits = append(allUnits, p.apps[a.Name].units...)
	}
	return allUnits, nil
}

func (p *FakeProvisioner) UnitsMetrics(ctx context.Context, a *appTypes.App) ([]provTypes.UnitMetric, error) {
	if err := p.getError("UnitsMetrics"); err != nil {
		return nil, err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	var unitsMetrics []provTypes.UnitMetric
	for _, unit := range p.apps[a.Name].units {
		unitsMetrics = append(unitsMetrics, provTypes.UnitMetric{
			ID:     unit.ID,
			CPU:    "10m",
			Memory: "100Mi",
		})
	}
	return unitsMetrics, nil
}

func (p *FakeProvisioner) MockRoutableAddresses(app *appTypes.App, addrs []appTypes.RoutableAddresses) {
	p.mut.Lock()
	defer p.mut.Unlock()
	a := p.apps[app.Name]
	a.mockAddrs = addrs
	p.apps[app.Name] = a
}

func (p *FakeProvisioner) RoutableAddresses(ctx context.Context, app *appTypes.App) ([]appTypes.RoutableAddresses, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	a := p.apps[app.Name]
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

func (p *FakeProvisioner) Addr(app *appTypes.App) (string, error) {
	if err := p.getError("Addr"); err != nil {
		return "", err
	}

	addrs, err := routertest.FakeRouter.Addresses(context.TODO(), app)
	if err != nil {
		return "", err
	}
	if len(addrs) > 0 {
		return addrs[0], nil
	}
	return "", nil
}

func (p *FakeProvisioner) Stop(ctx context.Context, app *appTypes.App, process string, version appTypes.AppVersion, w io.Writer) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.Name]
	if !ok {
		return errNotProvisioned
	}
	pApp.stops[process]++
	for i, u := range pApp.units {
		u.Status = provTypes.UnitStatusStopped
		pApp.units[i] = u
	}
	p.apps[app.Name] = pApp
	return nil
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

func (p *FakeProvisioner) FilterAppsByUnitStatus(ctx context.Context, apps []*appTypes.App, status []string) ([]*appTypes.App, error) {
	filteredApps := []*appTypes.App{}
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

func (p *FakeProvisioner) UpdateApp(ctx context.Context, old, new *appTypes.App, w io.Writer) error {
	provApp := p.apps[old.Name]
	provApp.app = new
	if new.Pool != old.Pool {
		provApp.restarts[""]++
		provApp.restartsByVersion[""]++
	}
	p.apps[old.Name] = provApp
	return nil
}

func (p *FakeProvisioner) InternalAddresses(ctx context.Context, a *appTypes.App) ([]appTypes.AppInternalAddress, error) {
	return []appTypes.AppInternalAddress{
		{
			Domain:     fmt.Sprintf("%s-web.fake-cluster.local", a.Name),
			Port:       80,
			TargetPort: 8080,
			Protocol:   "TCP",
			Process:    "web",
		},
		{
			Domain:     fmt.Sprintf("%s-logs.fake-cluster.local", a.Name),
			Port:       12201,
			TargetPort: 12201,
			Protocol:   "UDP",
			Process:    "logs",
		},
		{
			Domain:     fmt.Sprintf("%s-logs-v2.fake-cluster.local", a.Name),
			Port:       12201,
			TargetPort: 12201,
			Protocol:   "UDP",
			Process:    "logs",
			Version:    "2",
		},
		{
			Domain:     fmt.Sprintf("%s-web-v2.fake-cluster.local", a.Name),
			Port:       80,
			TargetPort: 8080,
			Protocol:   "TCP",
			Process:    "web",
			Version:    "2",
		},
	}, nil
}

func (p *FakeProvisioner) ListLogs(ctx context.Context, obj *logTypes.LogabbleObject, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	if !p.LogsEnabled {
		return nil, provision.ErrLogsUnavailable
	}
	return []appTypes.Applog{
		{
			Message: "Fake message from provisioner",
		},
	}, nil
}

func (p *FakeProvisioner) WatchLogs(ctx context.Context, obj *logTypes.LogabbleObject, args appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
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

type provisionedApp struct {
	units     []provTypes.Unit
	app       *appTypes.App
	restarts  map[string]int
	starts    map[string]int
	stops     map[string]int
	unitLen   int
	lastData  map[string]interface{}
	image     string
	mockAddrs []appTypes.RoutableAddresses

	restartsByVersion map[string]int
}

type provisionedJob struct {
	units      []provTypes.Unit
	job        *jobTypes.Job
	executions int
}

type AutoScaleProvisioner struct {
	*FakeProvisioner
	autoscales map[string][]provTypes.AutoScaleSpec
}

var _ provision.AutoScaleProvisioner = &AutoScaleProvisioner{}

func (p *AutoScaleProvisioner) GetAutoScale(ctx context.Context, app *appTypes.App) ([]provTypes.AutoScaleSpec, error) {
	if p.autoscales == nil {
		return nil, nil
	}
	return p.autoscales[app.Name], nil
}

func (p *AutoScaleProvisioner) GetVerticalAutoScaleRecommendations(ctx context.Context, app *appTypes.App) ([]provTypes.RecommendedResources, error) {
	if p.autoscales == nil {
		return nil, nil
	}
	return []provTypes.RecommendedResources{
		{Process: "p1", Recommendations: []provTypes.RecommendedProcessResources{{Type: "target", CPU: "100m", Memory: "100MiB"}}},
	}, nil
}

func (p *AutoScaleProvisioner) SetAutoScale(ctx context.Context, app *appTypes.App, spec provTypes.AutoScaleSpec) error {
	if p.autoscales == nil {
		p.autoscales = make(map[string][]provTypes.AutoScaleSpec)
	}
	// Update existing autoscale for the process if it exists, otherwise append
	updated := false
	for i, existing := range p.autoscales[app.Name] {
		if existing.Process == spec.Process {
			p.autoscales[app.Name][i] = spec
			updated = true
			break
		}
	}
	if !updated {
		p.autoscales[app.Name] = append(p.autoscales[app.Name], spec)
	}
	return nil
}

func (p *AutoScaleProvisioner) SwapAutoScale(ctx context.Context, a *appTypes.App, versionStr string) error {
	version, _ := strconv.Atoi(versionStr)
	for i := range p.autoscales[a.Name] {
		p.autoscales[a.Name][i].Version = version
	}
	return nil
}

func (p *AutoScaleProvisioner) RemoveAutoScale(ctx context.Context, app *appTypes.App, process string) error {
	if p.autoscales == nil {
		return nil
	}
	previous := p.autoscales[app.Name]
	p.autoscales[app.Name] = nil
	for _, spec := range previous {
		if spec.Process == process {
			continue
		}
		p.autoscales[app.Name] = append(p.autoscales[app.Name], spec)
	}
	return nil
}

type JobProvisioner struct {
	*FakeProvisioner
}

var _ provision.JobProvisioner = &JobProvisioner{}

// JobUnits returns information about units related to a specific Job or CronJob
func (p *JobProvisioner) JobUnits(ctx context.Context, job *jobTypes.Job) ([]provTypes.Unit, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	return p.jobs[job.Name].units, nil
}

// JobSchedule creates a cronjob object in the cluster
func (p *JobProvisioner) EnsureJob(ctx context.Context, job *jobTypes.Job) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	name := job.Name
	p.jobs[name] = &provisionedJob{
		units: []provTypes.Unit{},
		job:   job,
	}
	return nil
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

func (p *JobProvisioner) TriggerCron(ctx context.Context, job *jobTypes.Job, pool string) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	j, ok := p.jobs[job.Name]
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
		units: []provTypes.Unit{
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
