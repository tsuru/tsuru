// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new provisioner on tsuru.
package provision

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/event"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	logTypes "github.com/tsuru/tsuru/types/log"
	provTypes "github.com/tsuru/tsuru/types/provision"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	"github.com/tsuru/tsuru/validation"

	_ "github.com/tsuru/tsuru/router/api"
)

const (
	defaultKubernetesProvisioner = "kubernetes"
	DefaultHealthcheckScheme     = "http"

	PoolMetadataName = "pool"
	WebProcessName   = "web"
)

var (
	ErrEmptyApp     = errors.New("no units for this app")
	ErrNodeNotFound = errors.New("node not found")

	ErrLogsUnavailable = errors.New("logs from provisioner are unavailable")
	DefaultProvisioner = defaultKubernetesProvisioner
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

	// GetPlatform returns the platform (type) of the app. It is equivalent
	// to the Unit `Type` field.
	GetPlatform() string

	// GetPlatformVersion returns the locked platform version of the app.
	GetPlatformVersion() string

	// GetDeploy returns the deploys that an app has.
	GetDeploys() uint

	Envs() map[string]bindTypes.EnvVar

	GetUpdatePlatform() bool

	GetRouters() []appTypes.AppRouter

	GetTeamOwner() string
	GetTeamsName() []string

	GetPlan() appTypes.Plan
	GetPool() string
	GetProcess(process string) *appTypes.Process

	ListTags() []string

	GetMetadata(process string) appTypes.Metadata

	GetRegistry(context.Context) (imgTypes.ImageRegistry, error)
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
	Units(context.Context, ...App) ([]provTypes.Unit, error)

	// RoutableAddresses returns the addresses used to access an application.
	RoutableAddresses(context.Context, App) ([]appTypes.RoutableAddresses, error)
}

type JobProvisioner interface {
	// JobUnits returns information about units related to a specific Job or CronJob
	JobUnits(context.Context, *jobTypes.Job) ([]provTypes.Unit, error)

	// EnsureJob creates/update a cronjob object in the cluster
	EnsureJob(context.Context, *jobTypes.Job) error

	DestroyJob(context.Context, *jobTypes.Job) error
	TriggerCron(ctx context.Context, name, pool string) error
	KillJobUnit(ctx context.Context, job *jobTypes.Job, unitName string, force bool) error
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
	ListLogs(ctx context.Context, obj logTypes.LogabbleObject, args appTypes.ListLogArgs) ([]appTypes.Applog, error)
	WatchLogs(ctx context.Context, obj logTypes.LogabbleObject, args appTypes.ListLogArgs) (appTypes.LogWatcher, error)
}

// MetricsProvisioner is a provisioner that have capability to view metrics of workloads
type MetricsProvisioner interface {
	// Units returns information about cpu and memory usage by App.
	UnitsMetrics(ctx context.Context, a App) ([]provTypes.UnitMetric, error)
}

// UpdatableProvisioner is a provisioner that stores data about applications
// and must be notified when they are updated
type UpdatableProvisioner interface {
	UpdateApp(ctx context.Context, old, new App, w io.Writer) error
}

// InterAppProvisioner is a provisioner that allows an app to comunicate with each other
// using internal dns and own load balancers provided by provisioner.
type InterAppProvisioner interface {
	InternalAddresses(ctx context.Context, a App) ([]appTypes.AppInternalAddress, error)
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

type KillUnitProvisioner interface {
	KillUnit(ctx context.Context, app App, unit string, force bool) error
}

// HCProvisioner is a provisioner that may handle loadbalancing healthchecks.
type HCProvisioner interface {
	// HandlesHC returns true if the provisioner will handle healthchecking
	// instead of the router.
	HandlesHC() bool
}

type RebalanceNodesOptions struct {
	Event          *event.Event
	Pool           string
	MetadataFilter map[string]string
	AppFilter      []string
	Dry            bool
	Force          bool
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

func CPUValueOfAutoScaleSpec(s *provTypes.AutoScaleSpec, a App) (int, error) {
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

	cpuLimit := a.GetPlan().GetMilliCPU()
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

func ValidateAutoScaleSpec(s *provTypes.AutoScaleSpec, quotaLimit int, a App) error {
	if s.MinUnits == 0 {
		return errors.New("minimum units must be greater than 0")
	}
	if s.MaxUnits <= s.MinUnits {
		return errors.New("maximum units must be greater than minimum units")
	}
	if quotaLimit > 0 && s.MaxUnits > uint(quotaLimit) {
		return errors.New("maximum units cannot be greater than quota limit")
	}
	if s.AverageCPU == "" && len(s.Schedules) == 0 && len(s.Prometheus) == 0 {
		return errors.New("you have to configure at least one trigger between cpu, schedule and prometheus")
	}
	if s.AverageCPU != "" {
		_, err := CPUValueOfAutoScaleSpec(s, a)
		if err != nil {
			return err
		}
	}

	for _, prometheus := range s.Prometheus {
		if !validation.ValidateName(prometheus.Name) {
			return fmt.Errorf("\"%s\" is an invalid name, it must contain only lower case letters, numbers or dashes and starts with a letter", prometheus.Name)
		}

		if prometheus.Threshold <= 0 {
			return fmt.Errorf("prometheus threshold of name %q must be greater than 0", prometheus.Name)
		}
	}
	return nil
}

type AutoScaleProvisioner interface {
	GetAutoScale(ctx context.Context, a App) ([]provTypes.AutoScaleSpec, error)
	GetVerticalAutoScaleRecommendations(ctx context.Context, a App) ([]provTypes.RecommendedResources, error)
	SetAutoScale(ctx context.Context, a App, spec provTypes.AutoScaleSpec) error
	RemoveAutoScale(ctx context.Context, a App, process string) error
}

type UnitStatusData struct {
	ID     string
	Name   string
	Status provTypes.UnitStatus
}

type NodeCheckResult struct {
	Name       string
	Err        string
	Successful bool
}

type MultiRegistryProvisioner interface {
	RegistryForPool(ctx context.Context, pool string) (imgTypes.ImageRegistry, error)
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
		DefaultProvisioner = defaultKubernetesProvisioner
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
