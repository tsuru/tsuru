package deploy

import "io"

type App interface {
	// Command executes a command in the application units
	Command(io.Writer, io.Writer, ...string) error

	// Restart restarts the application process
	Restart(io.Writer) error

	// InstallDeps run the dependencies installation hook
	InstallDeps(io.Writer) error

	GetName() string
}

type Deployer interface {
	Deploy(App, io.Writer) error
}
