// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provisiontest

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/router/routertest"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/quota"
)

var (
	ProvisionerInstance *FakeProvisioner
	errNotProvisioned         = &provision.Error{Reason: "App is not provisioned."}
	uniqueIpCounter     int32 = 0

	_ provision.NodeProvisioner      = &FakeProvisioner{}
	_ provision.UpdatableProvisioner = &FakeProvisioner{}
	_ provision.Provisioner          = &FakeProvisioner{}
	_ provision.App                  = &FakeApp{}
	_ bind.App                       = &FakeApp{}
)

const fakeAppImage = "app-image"

func init() {
	ProvisionerInstance = NewFakeProvisioner()
	provision.Register("fake", func() (provision.Provisioner, error) {
		return ProvisionerInstance, nil
	})
}

// Fake implementation for provision.App.
type FakeApp struct {
	name           string
	cname          []string
	IP             string
	platform       string
	units          []provision.Unit
	logs           []string
	logMut         sync.Mutex
	Commands       []string
	Memory         int64
	Swap           int64
	CpuShare       int
	commMut        sync.Mutex
	Deploys        uint
	env            map[string]bind.EnvVar
	bindCalls      []*provision.Unit
	bindLock       sync.Mutex
	serviceEnvs    []bind.ServiceEnvVar
	serviceLock    sync.Mutex
	Pool           string
	UpdatePlatform bool
	TeamOwner      string
	Teams          []string
	Quota          quota.Quota
}

func NewFakeApp(name, platform string, units int) *FakeApp {
	return NewFakeAppWithPool(name, platform, "test-default", units)
}

func NewFakeAppWithPool(name, platform, pool string, units int) *FakeApp {
	app := FakeApp{
		name:     name,
		platform: platform,
		units:    make([]provision.Unit, units),
		Quota:    quota.UnlimitedQuota,
		Pool:     pool,
	}
	routertest.FakeRouter.AddBackend(&app)
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

func (a *FakeApp) GetMemory() int64 {
	return a.Memory
}

func (a *FakeApp) GetSwap() int64 {
	return a.Swap
}

func (a *FakeApp) GetCpuShare() int {
	return a.CpuShare
}

func (a *FakeApp) GetTeamsName() []string {
	return a.Teams
}

func (a *FakeApp) HasBind(unit *provision.Unit) bool {
	a.bindLock.Lock()
	defer a.bindLock.Unlock()
	for _, u := range a.bindCalls {
		if u.ID == unit.ID {
			return true
		}
	}
	return false
}

func (a *FakeApp) BindUnit(unit *provision.Unit) error {
	a.bindLock.Lock()
	defer a.bindLock.Unlock()
	a.bindCalls = append(a.bindCalls, unit)
	return nil
}

func (a *FakeApp) UnbindUnit(unit *provision.Unit) error {
	a.bindLock.Lock()
	defer a.bindLock.Unlock()
	index := -1
	for i, u := range a.bindCalls {
		if u.ID == unit.ID {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("not bound")
	}
	length := len(a.bindCalls)
	a.bindCalls[index] = a.bindCalls[length-1]
	a.bindCalls = a.bindCalls[:length-1]
	return nil
}

func (a *FakeApp) GetQuota() quota.Quota {
	return a.Quota
}

func (a *FakeApp) SetQuotaInUse(inUse int) error {
	if !a.Quota.IsUnlimited() && inUse > a.Quota.Limit {
		return &quota.QuotaExceededError{
			Requested: uint(inUse),
			Available: uint(a.Quota.Limit),
		}
	}
	a.Quota.InUse = inUse
	return nil
}

func (a *FakeApp) GetCname() []string {
	return a.cname
}

func (a *FakeApp) GetServiceEnvs() []bind.ServiceEnvVar {
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

func (a *FakeApp) GetPool() string {
	return a.Pool
}

func (a *FakeApp) GetPlatform() string {
	return a.platform
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

func (a *FakeApp) SetEnv(env bind.EnvVar) {
	if a.env == nil {
		a.env = map[string]bind.EnvVar{}
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

func (a *FakeApp) GetLock() provision.AppLock {
	return nil
}

func (a *FakeApp) GetUnits() ([]bind.Unit, error) {
	units := make([]bind.Unit, len(a.units))
	for i := range a.units {
		units[i] = &a.units[i]
	}
	return units, nil
}

func (a *FakeApp) Envs() map[string]bind.EnvVar {
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
	addr, err := routertest.FakeRouter.Addr(app.GetName())
	if err != nil {
		return nil, err
	}
	return []string{addr}, nil
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
	Name           string
	outputs        chan []byte
	failures       chan failure
	apps           map[string]provisionedApp
	mut            sync.RWMutex
	execs          map[string][]provision.ExecOptions
	execsMut       sync.Mutex
	nodes          map[string]FakeNode
	nodeContainers map[string]int
}

func NewFakeProvisioner() *FakeProvisioner {
	p := FakeProvisioner{Name: "fake"}
	p.outputs = make(chan []byte, 8)
	p.failures = make(chan failure, 8)
	p.apps = make(map[string]provisionedApp)
	p.execs = make(map[string][]provision.ExecOptions)
	p.nodes = make(map[string]FakeNode)
	p.nodeContainers = make(map[string]int)
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

type FakeNode struct {
	ID         string
	Addr       string
	PoolName   string
	Meta       map[string]string
	status     string
	p          *FakeProvisioner
	failures   int
	hasSuccess bool
}

func (n *FakeNode) IaaSID() string {
	return n.ID
}

func (n *FakeNode) Pool() string {
	return n.PoolName
}

func (n *FakeNode) Address() string {
	return n.Addr
}

func (n *FakeNode) Metadata() map[string]string {
	return n.Meta
}

func (n *FakeNode) MetadataNoPrefix() map[string]string {
	return n.Meta
}

func (n *FakeNode) Units() ([]provision.Unit, error) {
	n.p.mut.Lock()
	defer n.p.mut.Unlock()
	return n.unitsLocked()
}

func (n *FakeNode) unitsLocked() ([]provision.Unit, error) {
	var units []provision.Unit
	for _, a := range n.p.apps {
		for _, u := range a.units {
			if net.URLToHost(u.Address.String()) == net.URLToHost(n.Addr) {
				units = append(units, u)
			}
		}
	}
	return units, nil
}

func (n *FakeNode) Status() string {
	return n.status
}

func (n *FakeNode) FailureCount() int {
	return n.failures
}

func (n *FakeNode) HasSuccess() bool {
	return n.hasSuccess
}

func (n *FakeNode) ResetFailures() {
	n.failures = 0
}

func (n *FakeNode) Provisioner() provision.NodeProvisioner {
	return n.p
}

func (n *FakeNode) SetHealth(failures int, hasSuccess bool) {
	n.failures = failures
	n.hasSuccess = hasSuccess
}

func (p *FakeProvisioner) AddNode(opts provision.AddNodeOptions) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	if err := p.getError("AddNode"); err != nil {
		return err
	}
	if err := p.getError("AddNode:" + opts.Address); err != nil {
		return err
	}
	metadata := opts.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	if _, ok := p.nodes[opts.Address]; ok {
		return errors.New("fake node already exists")
	}
	p.nodes[opts.Address] = FakeNode{
		ID:       opts.IaaSID,
		Addr:     opts.Address,
		PoolName: opts.Pool,
		Meta:     metadata,
		p:        p,
		status:   "enabled",
	}
	return nil
}

func (p *FakeProvisioner) GetNode(address string) (provision.Node, error) {
	p.mut.RLock()
	defer p.mut.RUnlock()
	if err := p.getError("GetNode"); err != nil {
		return nil, err
	}
	if n, ok := p.nodes[address]; ok {
		return &n, nil
	}
	return nil, provision.ErrNodeNotFound
}

func (p *FakeProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	if err := p.getError("RemoveNode"); err != nil {
		return err
	}
	_, ok := p.nodes[opts.Address]
	if !ok {
		return provision.ErrNodeNotFound
	}
	delete(p.nodes, opts.Address)
	if opts.Writer != nil {
		if opts.Rebalance {
			opts.Writer.Write([]byte("rebalancing..."))
			p.rebalanceNodesLocked(provision.RebalanceNodesOptions{
				Force: true,
			})
		}
		opts.Writer.Write([]byte("remove done!"))
	}
	return nil
}

func (p *FakeProvisioner) UpdateNode(opts provision.UpdateNodeOptions) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	if err := p.getError("UpdateNode"); err != nil {
		return err
	}
	n, ok := p.nodes[opts.Address]
	if !ok {
		return provision.ErrNodeNotFound
	}
	if opts.Pool != "" {
		n.PoolName = opts.Pool
	}
	if opts.Metadata != nil {
		n.Meta = opts.Metadata
	}
	if opts.Enable {
		n.status = "enabled"
	}
	if opts.Disable {
		n.status = "disabled"
	}
	p.nodes[opts.Address] = n
	return nil
}

type nodeList []provision.Node

func (l nodeList) Len() int           { return len(l) }
func (l nodeList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l nodeList) Less(i, j int) bool { return l[i].Address() < l[j].Address() }

func (p *FakeProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	p.mut.RLock()
	defer p.mut.RUnlock()
	if err := p.getError("ListNodes"); err != nil {
		return nil, err
	}
	var result []provision.Node
	if addressFilter != nil {
		result = make([]provision.Node, 0, len(addressFilter))
		for _, a := range addressFilter {
			n := p.nodes[a]
			result = append(result, &n)
		}
	} else {
		result = make([]provision.Node, 0, len(p.nodes))
		for a := range p.nodes {
			n := p.nodes[a]
			result = append(result, &n)
		}
	}
	sort.Sort(nodeList(result))
	return result, nil
}

func (p *FakeProvisioner) NodeForNodeData(nodeData provision.NodeStatusData) (provision.Node, error) {
	nodeAddrMap := map[string]provision.Node{}
	for addr, n := range p.nodes {
		n := n
		nodeAddrMap[net.URLToHost(addr)] = &n
	}
	var node provision.Node
	for _, addr := range nodeData.Addrs {
		n := nodeAddrMap[net.URLToHost(addr)]
		if n != nil {
			if node != nil {
				return nil, errors.Errorf("addrs match multiple nodes: %v", nodeData.Addrs)
			}
			node = n
		}
	}
	if node == nil {
		return nil, provision.ErrNodeNotFound
	}
	return node, nil
}

func (p *FakeProvisioner) RebalanceNodes(opts provision.RebalanceNodesOptions) (bool, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	return p.rebalanceNodesLocked(opts)
}

func (p *FakeProvisioner) rebalanceNodesLocked(opts provision.RebalanceNodesOptions) (bool, error) {
	if err := p.getError("RebalanceNodes"); err != nil {
		return true, err
	}
	var w io.Writer
	if opts.Event == nil {
		w = ioutil.Discard
	} else {
		w = opts.Event
	}
	fmt.Fprintf(w, "rebalancing - dry: %v, force: %v\n", opts.Dry, opts.Force)
	if len(opts.AppFilter) != 0 {
		fmt.Fprintf(w, "filtering apps: %v\n", opts.AppFilter)
	}
	if len(opts.MetadataFilter) != 0 {
		fmt.Fprintf(w, "filtering metadata: %v\n", opts.MetadataFilter)
	}
	if opts.Pool != "" {
		fmt.Fprintf(w, "filtering pool: %v\n", opts.Pool)
	}
	if len(p.nodes) == 0 || opts.Dry {
		return true, nil
	}
	max := 0
	min := -1
	var nodes []FakeNode
	for _, n := range p.nodes {
		nodes = append(nodes, n)
		units, err := n.unitsLocked()
		if err != nil {
			return true, err
		}
		unitCount := len(units)
		if unitCount > max {
			max = unitCount
		}
		if min == -1 || unitCount < min {
			min = unitCount
		}
	}
	if max-min < 2 && !opts.Force {
		return false, nil
	}
	gi := 0
	for _, a := range p.apps {
		nodeIdx := 0
		for i := range a.units {
			u := &a.units[i]
			firstIdx := nodeIdx
			var hostAddr string
			for {
				idx := nodeIdx
				nodeIdx = (nodeIdx + 1) % len(nodes)
				if nodes[idx].Pool() == a.app.GetPool() {
					hostAddr = net.URLToHost(nodes[idx].Address())
					break
				}
				if nodeIdx == firstIdx {
					return true, errors.Errorf("unable to find node for pool %s", a.app.GetPool())
				}
			}
			u.IP = hostAddr
			u.Address = &url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", hostAddr, gi),
			}
			gi++
		}
	}
	return true, nil
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

// Sleeps returns the number of sleeps for a given app.
func (p *FakeProvisioner) Sleeps(app provision.App, process string) int {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.apps[app.GetName()].sleeps[process]
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

	p.execsMut.Lock()
	p.execs = make(map[string][]provision.ExecOptions)
	p.execsMut.Unlock()

	p.mut.Lock()
	p.nodes = make(map[string]FakeNode)
	p.mut.Unlock()
	uniqueIpCounter = 0

	p.nodeContainers = make(map[string]int)

	for {
		select {
		case <-p.outputs:
		case <-p.failures:
		default:
			return
		}
	}
}

func (p *FakeProvisioner) Swap(app1, app2 provision.App, cnameOnly bool) error {
	return routertest.FakeRouter.Swap(app1.GetName(), app2.GetName(), cnameOnly)
}

func (p *FakeProvisioner) Deploy(app provision.App, img string, evt *event.Event) (string, error) {
	if err := p.getError("Deploy"); err != nil {
		return "", err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return "", errNotProvisioned
	}
	pApp.image = img
	evt.Write([]byte("Builder deploy called"))
	p.apps[app.GetName()] = pApp
	return fakeAppImage, nil
}

func (p *FakeProvisioner) GetClient(app provision.App) (provision.BuilderDockerClient, error) {
	for _, node := range p.nodes {
		client, err := docker.NewClient(node.Addr)
		if err != nil {
			return nil, err
		}
		return &dockercommon.PullAndCreateClient{Client: client}, nil
	}
	return nil, errors.New("No node found")

}

func (p *FakeProvisioner) CleanImage(appName, imgName string) error {
	for _, node := range p.nodes {
		c, err := docker.NewClient(node.Addr)
		if err != nil {
			return err
		}
		err = c.RemoveImage(imgName)
		if err != nil && err != docker.ErrNoSuchImage {
			return err
		}
	}
	return nil
}

func (p *FakeProvisioner) ArchiveDeploy(app provision.App, archiveURL string, evt *event.Event) (string, error) {
	if err := p.getError("ArchiveDeploy"); err != nil {
		return "", err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return "", errNotProvisioned
	}
	evt.Write([]byte("Archive deploy called"))
	pApp.lastArchive = archiveURL
	p.apps[app.GetName()] = pApp
	return fakeAppImage, nil
}

func (p *FakeProvisioner) UploadDeploy(app provision.App, file io.ReadCloser, fileSize int64, build bool, evt *event.Event) (string, error) {
	if err := p.getError("UploadDeploy"); err != nil {
		return "", err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return "", errNotProvisioned
	}
	evt.Write([]byte("Upload deploy called"))
	pApp.lastFile = file
	p.apps[app.GetName()] = pApp
	return fakeAppImage, nil
}

func (p *FakeProvisioner) ImageDeploy(app provision.App, img string, evt *event.Event) (string, error) {
	if err := p.getError("ImageDeploy"); err != nil {
		return "", err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return "", errNotProvisioned
	}
	pApp.image = img
	evt.Write([]byte("Image deploy called"))
	p.apps[app.GetName()] = pApp
	return img, nil
}

func (p *FakeProvisioner) Rollback(app provision.App, img string, evt *event.Event) (string, error) {
	if err := p.getError("Rollback"); err != nil {
		return "", err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return "", errNotProvisioned
	}
	evt.Write([]byte("Rollback deploy called"))
	p.apps[app.GetName()] = pApp
	return img, nil
}

func (p *FakeProvisioner) Rebuild(app provision.App, evt *event.Event) (string, error) {
	if err := p.getError("Rebuild"); err != nil {
		return "", err
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return "", errNotProvisioned
	}
	evt.Write([]byte("Rebuild deploy called"))
	p.apps[app.GetName()] = pApp
	return fakeAppImage, nil
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
		app:      app,
		restarts: make(map[string]int),
		starts:   make(map[string]int),
		stops:    make(map[string]int),
		sleeps:   make(map[string]int),
	}
	return nil
}

func (p *FakeProvisioner) Restart(app provision.App, process string, w io.Writer) error {
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

func (p *FakeProvisioner) Start(app provision.App, process string) error {
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

func (p *FakeProvisioner) AddUnits(app provision.App, n uint, process string, w io.Writer) error {
	_, err := p.AddUnitsToNode(app, n, process, w, "")
	return err
}

func (p *FakeProvisioner) AddUnitsToNode(app provision.App, n uint, process string, w io.Writer, nodeAddr string) ([]provision.Unit, error) {
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
	for i := uint(0); i < n; i++ {
		val := atomic.AddInt32(&uniqueIpCounter, 1)
		var hostAddr string
		if nodeAddr != "" {
			hostAddr = net.URLToHost(nodeAddr)
		} else if len(p.nodes) > 0 {
			for _, n := range p.nodes {
				hostAddr = net.URLToHost(n.Address())
				break
			}
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
		}
		addresses = append(addresses, unit.Address)
		pApp.units = append(pApp.units, unit)
		pApp.unitLen++
	}
	err := routertest.FakeRouter.AddRoutes(name, addresses)
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

func (p *FakeProvisioner) RemoveUnits(app provision.App, n uint, process string, w io.Writer) error {
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
	err := routertest.FakeRouter.RemoveRoutes(app.GetName(), addresses)
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

func (p *FakeProvisioner) Units(apps ...provision.App) ([]provision.Unit, error) {
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

func (p *FakeProvisioner) RoutableAddresses(app provision.App) ([]url.URL, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	units := p.apps[app.GetName()].units
	addrs := make([]url.URL, len(units))
	for i := range units {
		addrs[i] = *units[i].Address
	}
	return addrs, nil
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
	return routertest.FakeRouter.Addr(app.GetName())
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
	return routertest.FakeRouter.SetCName(cname, app.GetName())
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
	return routertest.FakeRouter.UnsetCName(cname, app.GetName())
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

func (p *FakeProvisioner) Stop(app provision.App, process string) error {
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

func (p *FakeProvisioner) Sleep(app provision.App, process string) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	pApp, ok := p.apps[app.GetName()]
	if !ok {
		return errNotProvisioned
	}
	pApp.sleeps[process]++
	for i, u := range pApp.units {
		u.Status = provision.StatusAsleep
		pApp.units[i] = u
	}
	p.apps[app.GetName()] = pApp
	return nil
}

func (p *FakeProvisioner) RegisterUnit(a provision.App, unitId string, customData map[string]interface{}) error {
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

func (p *FakeProvisioner) ExecuteCommand(opts provision.ExecOptions) error {
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

func (p *FakeProvisioner) FilterAppsByUnitStatus(apps []provision.App, status []string) ([]provision.App, error) {
	filteredApps := []provision.App{}
	for i := range apps {
		units, _ := p.Units(apps[i])
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

func (p *FakeProvisioner) UpgradeNodeContainer(name string, pool string, writer io.Writer) error {
	p.nodeContainers[name+"-"+pool]++
	return nil
}

func (p *FakeProvisioner) RemoveNodeContainer(name string, pool string, writer io.Writer) error {
	p.nodeContainers[name+"-"+pool] = 0
	return nil
}

func (p *FakeProvisioner) HasNodeContainer(name string, pool string) bool {
	return p.nodeContainers[name+"-"+pool] > 0
}

func (p *FakeProvisioner) DeleteVolume(volName, pool string) error {
	return nil
}

func (p *FakeProvisioner) IsVolumeProvisioned(name, pool string) (bool, error) {
	return false, nil
}

func (p *FakeProvisioner) UpdateApp(old, new provision.App, w io.Writer) error {
	provApp := p.apps[old.GetName()]
	provApp.app = new
	p.apps[old.GetName()] = provApp
	return nil
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
	units       []provision.Unit
	app         provision.App
	restarts    map[string]int
	starts      map[string]int
	stops       map[string]int
	sleeps      map[string]int
	lastArchive string
	lastFile    io.ReadCloser
	cnames      []string
	unitLen     int
	lastData    map[string]interface{}
	image       string
}
