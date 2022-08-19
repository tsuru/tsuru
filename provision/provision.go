// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new provisioner on tsuru.
package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/event"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	provTypes "github.com/tsuru/tsuru/types/provision"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
)

const (
	defaultDockerProvisioner = "docker"
	DefaultHealthcheckScheme = "http"

	PoolMetadataName   = "pool"
	IaaSIDMetadataName = "iaas-id"
	IaaSMetadataName   = "iaas"
	WebProcessName     = "web"
)

var (
	ErrInvalidStatus = errors.New("invalid status")
	ErrEmptyApp      = errors.New("no units for this app")
	ErrNodeNotFound  = errors.New("node not found")

	ErrLogsUnavailable = errors.New("logs from provisioner are unavailable")
	DefaultProvisioner = defaultDockerProvisioner
)

type UnitNotFoundError struct {
	ID string
}

func (e *UnitNotFoundError) Error() string {
	return fmt.Sprintf("unit %q not found", e.ID)
}

type InvalidProcessError struct {
	Msg string
}

func (e InvalidProcessError) Error() string {
	return fmt.Sprintf("process error: %s", e.Msg)
}

type ProvisionerNotSupported struct {
	Prov   Provisioner
	Action string
}

func (e ProvisionerNotSupported) Error() string {
	return fmt.Sprintf("provisioner %q does not support %s", e.Prov.GetName(), e.Action)
}

// Status represents the status of a unit in tsuru.
type Status string

func (s Status) String() string {
	return string(s)
}

func ParseStatus(status string) (Status, error) {
	switch status {
	case "created":
		return StatusCreated, nil
	case "building":
		return StatusBuilding, nil
	case "error":
		return StatusError, nil
	case "started":
		return StatusStarted, nil
	case "starting":
		return StatusStarting, nil
	case "stopped":
		return StatusStopped, nil
	case "asleep":
		return StatusAsleep, nil
	}
	return Status(""), ErrInvalidStatus
}

//     Flow:
//                                    +----------------------------------------------+
//                                    |                                              |
//                                    |            Start                             |
//     +----------+                   |                      +---------+             |
//     | Building |                   +---------------------+| Stopped |             |
//     +----------+                   |                      +---------+             |
//           ^                        |                           ^                  |
//           |                        |                           |                  |
//      deploy unit                   |                         Stop                 |
//           |                        |                           |                  |
//           +                        v       RegisterUnit        +                  +
//      +---------+  app unit   +----------+  SetUnitStatus  +---------+  Sleep  +--------+
//      | Created | +---------> | Starting | +-------------> | Started |+------->| Asleep |
//      +---------+             +----------+                 +---------+         +--------+
//                                    +                         ^ +
//                                    |                         | |
//                              SetUnitStatus                   | |
//                                    |                         | |
//                                    v                         | |
//                                +-------+     SetUnitStatus   | |
//                                | Error | +-------------------+ |
//                                +-------+ <---------------------+
const (
	// StatusCreated is the initial status of a unit in the database,
	// it should transition shortly to a more specific status
	StatusCreated = Status("created")

	// StatusBuilding is the status for units being provisioned by the
	// provisioner, like in the deployment.
	StatusBuilding = Status("building")

	// StatusError is the status for units that failed to start, because of
	// an application error.
	StatusError = Status("error")

	// StatusStarting is set when the container is started in docker.
	StatusStarting = Status("starting")

	// StatusStarted is for cases where the unit is up and running, and bound
	// to the proper status, it's set by RegisterUnit and SetUnitStatus.
	StatusStarted = Status("started")

	// StatusStopped is for cases where the unit has been stopped.
	StatusStopped = Status("stopped")

	// StatusAsleep is for cases where the unit has been asleep.
	StatusAsleep = Status("asleep")
)

// Unit represents a provision unit. Can be a machine, container or anything
// IP-addressable.
type Unit struct {
	ID          string
	Name        string
	AppName     string
	ProcessName string
	Type        string
	IP          string
	Status      Status
	Address     *url.URL
	Addresses   []url.URL
	Version     int
	Routable    bool
	Restarts    *int32
	CreatedAt   *time.Time
	Ready       *bool
}

// GetName returns the name of the unit.
func (u *Unit) GetID() string {
	return u.ID
}

// GetIp returns the Unit.IP.
func (u *Unit) GetIp() string {
	return u.IP
}

func (u *Unit) MarshalJSON() ([]byte, error) {
	type UnitForMarshal Unit
	host, port, _ := net.SplitHostPort(u.Address.Host)
	// New fields added for compatibility with old routes returning containers.
	return json.Marshal(&struct {
		*UnitForMarshal
		HostAddr string
		HostPort string
		IP       string
	}{
		UnitForMarshal: (*UnitForMarshal)(u),
		HostAddr:       host,
		HostPort:       port,
		IP:             u.IP,
	})
}

// Available returns true if the unit is available. It will return true
// whenever the unit itself is available, even when the application process is
// not.
func (u *Unit) Available() bool {
	return u.Status == StatusStarted ||
		u.Status == StatusStarting ||
		u.Status == StatusError
}

// UnitMetric represents a a related metrics for an unit.
type UnitMetric struct {
	ID     string
	CPU    string
	Memory string
}

// Named is something that has a name, providing the GetName method.
type Named interface {
	GetName() string
}

// RunArgs groups together the arguments to run an App.
type RunArgs struct {
	Once     bool
	Isolated bool
}

// App represents a tsuru app.
//
// It contains only relevant information for provisioning.
type App interface {
	Named

	BindUnit(*Unit) error
	UnbindUnit(*Unit) error

	// GetPlatform returns the platform (type) of the app. It is equivalent
	// to the Unit `Type` field.
	GetPlatform() string

	// GetPlatformVersion returns the locked platform version of the app.
	GetPlatformVersion() string

	// GetDeploy returns the deploys that an app has.
	GetDeploys() uint

	Envs() map[string]bind.EnvVar

	GetMemory() int64
	GetMilliCPU() int
	GetSwap() int64
	GetCpuShare() int

	GetUpdatePlatform() bool

	GetRouters() []appTypes.AppRouter

	GetPool() string

	GetTeamOwner() string
	GetTeamsName() []string

	ListTags() []string

	GetMetadata() appTypes.Metadata

	GetRegistry() (imgTypes.ImageRegistry, error)
}

type BuilderDockerClient interface {
	PullAndCreateContainer(opts docker.CreateContainerOptions, w io.Writer) (*docker.Container, string, error)
	RemoveContainer(opts docker.RemoveContainerOptions) error
	StartContainer(id string, hostConfig *docker.HostConfig) error
	StopContainer(id string, timeout uint) error
	InspectContainer(id string) (*docker.Container, error)
	CommitContainer(docker.CommitContainerOptions) (*docker.Image, error)
	DownloadFromContainer(string, docker.DownloadFromContainerOptions) error
	UploadToContainer(string, docker.UploadToContainerOptions) error
	AttachToContainerNonBlocking(opts docker.AttachToContainerOptions) (docker.CloseWaiter, error)
	AttachToContainer(opts docker.AttachToContainerOptions) error
	WaitContainer(id string) (int, error)

	BuildImage(opts docker.BuildImageOptions) error
	PushImage(docker.PushImageOptions, docker.AuthConfiguration) error
	InspectImage(string) (*docker.Image, error)
	TagImage(string, docker.TagImageOptions) error
	RemoveImage(name string) error
	ImageHistory(name string) ([]docker.ImageHistory, error)

	SetTimeout(timeout time.Duration)
}

type ExecDockerClient interface {
	CreateExec(opts docker.CreateExecOptions) (*docker.Exec, error)
	StartExec(execId string, opts docker.StartExecOptions) error
	ResizeExecTTY(execId string, height, width int) error
	InspectExec(execId string) (*docker.ExecInspect, error)
}

type InspectData struct {
	Image     docker.Image
	TsuruYaml provTypes.TsuruYamlData
	Procfile  string
}

type BuilderKubeClient interface {
	BuildPod(context.Context, App, *event.Event, io.Reader, appTypes.AppVersion) error
	BuildPlatformImages(ctx context.Context, opts appTypes.PlatformOptions) ([]string, error)
	ImageTagPushAndInspect(context.Context, App, *event.Event, string, appTypes.AppVersion) (InspectData, error)
	DownloadFromContainer(context.Context, App, *event.Event, string) (io.ReadCloser, error)
}

type DeployArgs struct {
	App              App
	Version          appTypes.AppVersion
	Event            *event.Event
	PreserveVersions bool
	OverrideVersions bool
}

// BuilderDeploy is a provisioner that allows deploy builded image.
type BuilderDeploy interface {
	Deploy(context.Context, DeployArgs) (string, error)
}

type BuilderDeployDockerClient interface {
	BuilderDeploy
	GetClient(App) (BuilderDockerClient, error)
}

type BuilderDeployKubeClient interface {
	BuilderDeploy
	GetClient(App) (BuilderKubeClient, error)
}

type VersionsProvisioner interface {
	ToggleRoutable(context.Context, App, appTypes.AppVersion, bool) error
	DeployedVersions(context.Context, App) ([]int, error)
}

// Provisioner is the basic interface of this package.
//
// Any tsuru provisioner must implement this interface in order to provision
// tsuru apps.
type Provisioner interface {
	Named

	// Provision is called when tsuru is creating the app.
	Provision(context.Context, App) error

	// Destroy is called when tsuru is destroying the app.
	Destroy(context.Context, App) error

	// DestroyVersion is called when tsuru is destroying an app version.
	DestroyVersion(context.Context, App, appTypes.AppVersion) error

	// AddUnits adds units to an app. The first parameter is the app, the
	// second is the number of units to be added.
	//
	// It returns a slice containing all added units
	AddUnits(context.Context, App, uint, string, appTypes.AppVersion, io.Writer) error

	// RemoveUnits "undoes" AddUnits, removing the given number of units
	// from the app.
	RemoveUnits(context.Context, App, uint, string, appTypes.AppVersion, io.Writer) error

	// Restart restarts the units of the application, with an optional
	// string parameter represeting the name of the process to start. When
	// the process is empty, Restart will restart all units of the
	// application.
	Restart(context.Context, App, string, appTypes.AppVersion, io.Writer) error

	// Start starts the units of the application, with an optional string
	// parameter representing the name of the process to start. When the
	// process is empty, Start will start all units of the application.
	Start(context.Context, App, string, appTypes.AppVersion, io.Writer) error

	// Stop stops the units of the application, with an optional string
	// parameter representing the name of the process to start. When the
	// process is empty, Stop will stop all units of the application.
	Stop(context.Context, App, string, appTypes.AppVersion, io.Writer) error

	// Units returns information about units by App.
	Units(context.Context, ...App) ([]Unit, error)

	// RoutableAddresses returns the addresses used to access an application.
	RoutableAddresses(context.Context, App) ([]appTypes.RoutableAddresses, error)

	// Register a unit after the container has been created or restarted.
	RegisterUnit(context.Context, App, string, map[string]interface{}) error
}

type ExecOptions struct {
	App    App
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	Width  int
	Height int
	Term   string
	Cmds   []string
	Units  []string
}

type ExecutableProvisioner interface {
	ExecuteCommand(ctx context.Context, opts ExecOptions) error
}

// LogsProvisioner is a provisioner that is self responsible for storage logs.
type LogsProvisioner interface {
	ListLogs(ctx context.Context, app appTypes.App, args appTypes.ListLogArgs) ([]appTypes.Applog, error)
	WatchLogs(ctx context.Context, app appTypes.App, args appTypes.ListLogArgs) (appTypes.LogWatcher, error)
}

// MetricsProvisioner is a provisioner that have capability to view metrics of workloads
type MetricsProvisioner interface {
	// Units returns information about cpu and memory usage by App.
	UnitsMetrics(ctx context.Context, a App) ([]UnitMetric, error)
}

// SleepableProvisioner is a provisioner that allows putting applications to
// sleep.
type SleepableProvisioner interface {
	// Sleep puts the units of the application to sleep, with an optional string
	// parameter representing the name of the process to sleep. When the
	// process is empty, Sleep will put all units of the application to sleep.
	Sleep(context.Context, App, string, appTypes.AppVersion) error
}

// UpdatableProvisioner is a provisioner that stores data about applications
// and must be notified when they are updated
type UpdatableProvisioner interface {
	UpdateApp(ctx context.Context, old, new App, w io.Writer) error
}

// InterAppProvisioner is a provisioner that allows an app to comunicate with each other
// using internal dns and own load balancers provided by provisioner.
type InterAppProvisioner interface {
	InternalAddresses(ctx context.Context, a App) ([]AppInternalAddress, error)
}

type AppInternalAddress struct {
	Domain   string
	Protocol string
	Port     int32
	Version  string
	Process  string
}

// MessageProvisioner is a provisioner that provides a welcome message for
// logging.
type MessageProvisioner interface {
	StartupMessage() (string, error)
}

// InitializableProvisioner is a provisioner that provides an initialization
// method that should be called when the app is started
type InitializableProvisioner interface {
	Initialize() error
}

// OptionalLogsProvisioner is a provisioner that allows optionally disabling
// logs for a given app.
type OptionalLogsProvisioner interface {
	// Checks if logs are enabled for given app.
	LogsEnabled(App) (bool, string, error)
}

// UnitStatusProvisioner is a provisioner that receive notifications about unit
// status changes.
type UnitStatusProvisioner interface {
	// SetUnitStatus changes the status of a unit.
	SetUnitStatus(Unit, Status) error
}

type KillUnitProvisioner interface {
	KillUnit(ctx context.Context, app App, unit string, force bool) error
}

// HCProvisioner is a provisioner that may handle loadbalancing healthchecks.
type HCProvisioner interface {
	// HandlesHC returns true if the provisioner will handle healthchecking
	// instead of the router.
	HandlesHC() bool
}

type AddNodeOptions struct {
	IaaSID     string
	Address    string
	Pool       string
	Metadata   map[string]string
	Register   bool
	CaCert     []byte
	ClientCert []byte
	ClientKey  []byte
	WaitTO     time.Duration
}

type RemoveNodeOptions struct {
	Address   string
	Rebalance bool
	Writer    io.Writer
}

type UpdateNodeOptions struct {
	Address  string
	Pool     string
	Metadata map[string]string
	Enable   bool
	Disable  bool
}

type NodeProvisioner interface {
	Named

	// ListNodes returns a list of all nodes registered in the provisioner.
	ListNodes(ctx context.Context, addressFilter []string) ([]Node, error)

	// ListNodesByFilters returns a list of filtered nodes by filter.
	ListNodesByFilter(ctx context.Context, filter *provTypes.NodeFilter) ([]Node, error)

	// GetNode retrieves an existing node by its address.
	GetNode(ctx context.Context, address string) (Node, error)

	// AddNode adds a new node in the provisioner.
	AddNode(context.Context, AddNodeOptions) error

	// RemoveNode removes an existing node.
	RemoveNode(context.Context, RemoveNodeOptions) error

	// UpdateNode can be used to enable/disable a node and update its metadata.
	UpdateNode(context.Context, UpdateNodeOptions) error

	// NodeForNodeData finds a node matching the received NodeStatusData.
	NodeForNodeData(context.Context, NodeStatusData) (Node, error)
}

type RebalanceNodesOptions struct {
	Event          *event.Event
	Pool           string
	MetadataFilter map[string]string
	AppFilter      []string
	Dry            bool
	Force          bool
}

type NodeRebalanceProvisioner interface {
	RebalanceNodes(context.Context, RebalanceNodesOptions) (bool, error)
}

type NodeContainerProvisioner interface {
	UpgradeNodeContainer(ctx context.Context, name string, pool string, writer io.Writer) error
	RemoveNodeContainer(ctx context.Context, name string, pool string, writer io.Writer) error
}

// UnitFinderProvisioner is a provisioner that allows finding a specific unit
// by its id. New provisioners should not implement this interface, this was
// only used during events format migration and is exclusive to docker
// provisioner.
type UnitFinderProvisioner interface {
	// GetAppFromUnitID returns an app from unit id
	GetAppFromUnitID(context.Context, string) (App, error)
}

// AppFilterProvisioner is a provisioner that allows filtering apps by the
// state of its units.
type AppFilterProvisioner interface {
	FilterAppsByUnitStatus(context.Context, []App, []string) ([]App, error)
}

type VolumeProvisioner interface {
	ValidateVolume(context.Context, *volumeTypes.Volume) error
	IsVolumeProvisioned(ctx context.Context, volumeName, pool string) (bool, error)
	DeleteVolume(ctx context.Context, volumeName, pool string) error
}

type CleanImageProvisioner interface {
	CleanImage(appName string, image string) error
}

type AutoScaleSpec struct {
	Process    string `json:"process"`
	MinUnits   uint   `json:"minUnits"`
	MaxUnits   uint   `json:"maxUnits"`
	AverageCPU string `json:"averageCPU"`
	Version    int    `json:"version"`
}

type RecommendedResources struct {
	Process         string                        `json:"process"`
	Recommendations []RecommendedProcessResources `json:"recommendations"`
}

type RecommendedProcessResources struct {
	Type   string `json:"type"`
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

func (s AutoScaleSpec) ToCPUValue(a App) (int, error) {
	rawCPU := strings.TrimSuffix(s.AverageCPU, "%")
	cpu, err := strconv.Atoi(rawCPU)
	if err != nil {
		rawCPU = strings.TrimSuffix(s.AverageCPU, "m")
		cpu, err = strconv.Atoi(rawCPU)
		if err != nil {
			return 0, errors.Errorf("unable to parse value %q as autoscale cpu percentage", s.AverageCPU)
		}
		cpu = cpu / 10
	}

	cpuLimit := a.GetMilliCPU()
	if cpuLimit == 0 {
		// No cpu limit is set in app, the AverageCPU value must be considered
		// as absolute milli cores and we cannot validate it.
		return cpu * 10, nil
	}

	if cpu > 95 {
		return 0, errors.New("autoscale cpu value cannot be greater than 95%")
	}

	if cpu < 20 {
		return 0, errors.New("autoscale cpu value cannot be less than 20%")
	}

	return cpu, nil
}

func (s AutoScaleSpec) Validate(quotaLimit int, a App) error {
	if s.MinUnits == 0 {
		return errors.New("minimum units must be greater than 0")
	}
	if s.MaxUnits <= s.MinUnits {
		return errors.New("maximum units must be greater than minimum units")
	}
	if quotaLimit > 0 && s.MaxUnits > uint(quotaLimit) {
		return errors.New("maximum units cannot be greater than quota limit")
	}
	_, err := s.ToCPUValue(a)
	if err != nil {
		return err
	}
	return nil
}

type AutoScaleProvisioner interface {
	GetAutoScale(ctx context.Context, a App) ([]AutoScaleSpec, error)
	GetVerticalAutoScaleRecommendations(ctx context.Context, a App) ([]RecommendedResources, error)
	SetAutoScale(ctx context.Context, a App, spec AutoScaleSpec) error
	RemoveAutoScale(ctx context.Context, a App, process string) error
}

type Node interface {
	Pool() string
	IaaSID() string
	Address() string
	Status() string

	// Metadata returns node metadata exclusively managed by tsuru
	Metadata() map[string]string
	Units() ([]Unit, error)
	Provisioner() NodeProvisioner

	// MetadataNoPrefix returns node metadata managed by tsuru without any
	// tsuru specific prefix. This can be used with iaas providers.
	MetadataNoPrefix() map[string]string
}

type NodeExtraData interface {
	// ExtraData returns node metadata not managed by tsuru, like metadata
	// added by external sources.
	ExtraData() map[string]string
}

type NodeHealthChecker interface {
	Node
	FailureCount() int
	HasSuccess() bool
	ResetFailures()
}

type NodeSpec struct {
	// BSON tag for bson serialized compatibility with cluster.Node
	Address     string `bson:"_id"`
	IaaSID      string
	Metadata    map[string]string
	Status      string
	Pool        string
	Provisioner string
}

func NodeToSpec(n Node) NodeSpec {
	metadata := map[string]string{}
	if extra, ok := n.(NodeExtraData); ok {
		for k, v := range extra.ExtraData() {
			metadata[k] = v
		}
	}
	for k, v := range n.Metadata() {
		metadata[k] = v
	}
	var provName string
	prov := n.Provisioner()
	if prov != nil {
		provName = prov.GetName()
	}
	return NodeSpec{
		Address:     n.Address(),
		IaaSID:      n.IaaSID(),
		Metadata:    metadata,
		Status:      n.Status(),
		Pool:        n.Pool(),
		Provisioner: provName,
	}
}

func NodeToJSON(n Node) ([]byte, error) {
	return json.Marshal(NodeToSpec(n))
}

type NodeStatusData struct {
	Addrs  []string
	Units  []UnitStatusData
	Checks []NodeCheckResult
}

type UnitStatusData struct {
	ID     string
	Name   string
	Status Status
}

type NodeCheckResult struct {
	Name       string
	Err        string
	Successful bool
}

type MultiRegistryProvisioner interface {
	RegistryForApp(ctx context.Context, a App) (imgTypes.ImageRegistry, error)
}

type provisionerFactory func() (Provisioner, error)

var provisioners = make(map[string]provisionerFactory)

// Register registers a new provisioner in the Provisioner registry.
func Register(name string, pFunc provisionerFactory) {
	provisioners[name] = pFunc
}

// Unregister unregisters a provisioner.
func Unregister(name string) {
	delete(provisioners, name)
}

// Get gets the named provisioner from the registry.
func Get(name string) (Provisioner, error) {
	pFunc, ok := provisioners[name]
	if !ok {
		return nil, errors.Errorf("unknown provisioner: %q", name)
	}
	return pFunc()
}

func GetDefault() (Provisioner, error) {
	if DefaultProvisioner == "" {
		DefaultProvisioner = defaultDockerProvisioner
	}
	return Get(DefaultProvisioner)
}

// Registry returns the list of registered provisioners.
func Registry() ([]Provisioner, error) {
	registry := make([]Provisioner, 0, len(provisioners))
	for _, pFunc := range provisioners {
		p, err := pFunc()
		if err != nil {
			return nil, err
		}
		registry = append(registry, p)
	}
	return registry, nil
}

func InitializeAll() error {
	provisioners, err := Registry()
	if err != nil {
		return err
	}
	var startupMessage string
	for _, p := range provisioners {
		if initializableProvisioner, ok := p.(InitializableProvisioner); ok {
			err = initializableProvisioner.Initialize()
			if err != nil {
				fmt.Printf("error initializing provisioner: %v\n", err)
			}
		}
		if messageProvisioner, ok := p.(MessageProvisioner); ok {
			startupMessage, err = messageProvisioner.StartupMessage()
			if err == nil && startupMessage != "" {
				fmt.Print(startupMessage)
			}
		}
	}
	return nil
}

// Error represents a provisioning error. It encapsulates further errors.
type Error struct {
	Reason string
	Err    error
}

// Error is the string representation of a provisioning error.
func (e *Error) Error() string {
	var err string
	if e.Err != nil {
		err = e.Err.Error() + ": " + e.Reason
	} else {
		err = e.Reason
	}
	return err
}

type ErrUnitStartup struct {
	CrashedUnits     []string
	CrashedUnitsLogs []appTypes.Applog
	Err              error
}

func (e ErrUnitStartup) Error() string {
	return e.Err.Error()
}

func (e ErrUnitStartup) Cause() error {
	return e.Err
}

func IsStartupError(err error) (*ErrUnitStartup, bool) {
	type causer interface {
		Cause() error
	}

	for err != nil {
		if errUnitStartup, ok := err.(ErrUnitStartup); ok {
			return &errUnitStartup, ok
		}
		if errUnitStartup, ok := err.(*ErrUnitStartup); ok {
			return errUnitStartup, ok
		}

		cause, ok := err.(causer)
		if !ok {
			break
		}
		err = cause.Cause()
	}

	return nil, false
}

func MainAppProcess(processes []string) string {
	if len(processes) == 0 {
		return ""
	}
	for _, p := range processes {
		if p == WebProcessName {
			return p
		}
	}
	sort.Strings(processes)
	return processes[0]
}
