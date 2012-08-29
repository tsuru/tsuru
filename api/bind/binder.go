package bind

import "fmt"

// AppContainer provides methods for a container of apps.
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

func (e *EnvVar) String() string {
	var value, suffix string
	if e.Public {
		value = e.Value
	} else {
		value = "***"
		suffix = " (private variable)"
	}
	return fmt.Sprintf("%s=%s%s", e.Name, value, suffix)
}

type Unit interface {
	GetIp() string
}

type App interface {
	GetName() string
	GetUnits() []Unit
	InstanceEnv(string) map[string]EnvVar
	SetEnvs([]EnvVar, bool) error
	UnsetEnvs([]string, bool) error
}

type Binder interface {
	AppContainer
	Bind(App) error
	Unbind(App) error
}
