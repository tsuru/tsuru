package builder

import (
	"io"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
)

const defaultBuilder = "docker"

type BuildOpts struct {
	BuildFromFile  bool
	Rebuild        bool
	ArchiveURL     string
	ArchiveFile    io.Reader
	ArchiveTarFile io.ReadCloser
	ArchiveSize    int64
}

// Builder is the basic interface of this package.
type Builder interface {
	Build(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts BuildOpts) (string, error)
}

var builders = make(map[string]Builder)

// Register registers a new builder in the Builder registry.
func Register(name string, builder Builder) {
	builders[name] = builder
}

// Get gets the named builder from the registry.
func Get(name string) (Builder, error) {
	b, ok := builders[name]
	if !ok {
		return nil, errors.Errorf("unknown builder: %q", name)
	}
	return b, nil
}

func GetDefault() (Builder, error) {
	return Get(defaultBuilder)
}
