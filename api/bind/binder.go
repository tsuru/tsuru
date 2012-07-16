package bind

import (
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
)

// AppContainer provides methdos for a container of apps.
//
// The container stores only the names of the apps.
type AppContainer interface {
	// Adds an app to the container.
	AddApp(string) error

	// Finds an app in the container, returning an index a value >= 0 if it is
	// present, and -1 if not present.
	FindApp(string) int

	// Removes an app form the container.
	RemoveApp(name string) error
}

type EnvVar struct {
	Name         string
	Value        string
	Public       bool
	InstanceName string
}

type App interface {
	CheckUserAccess(*auth.User) bool
	GetName() string
	GetUnits() []unit.Unit
	SetEnvs([]EnvVar, bool) error
	UnsetEnvs([]string, bool) error
}

type Binder interface {
	AppContainer
	Bind(App, *auth.User) error
	Unbind(App, *auth.User) error
}
