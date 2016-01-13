package vulcan

import (
	"encoding/json"
	"fmt"

	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/coreos/go-etcd/etcd"
)

const (
	endpointKey = "vulcand/upstreams/%v/endpoints/%v"
	locationKey = "vulcand/hosts/%v/locations/%v"

	backendPath  = "vulcand2/backends/%v"
	serverPath   = "vulcand2/backends/%v/servers/%v"
	frontendPath = "vulcand2/frontends/%v.%v"

	// If vulcand registration is enabled, the app will be re-registering itself every
	// this amount of seconds.
	endpointTTL = 5 // seconds
)

type Registry struct {
	etcdClient *etcd.Client
	config     Config
}

type Config struct {
	PublicAPIHost    string
	ProtectedAPIHost string
}

func NewRegistry(config Config) *Registry {
	return &Registry{
		etcdClient: etcd.NewClient([]string{"http://127.0.0.1:4001"}),
		config:     config,
	}
}

func (r *Registry) RegisterEndpoint(e *Endpoint) error {
	key := fmt.Sprintf(endpointKey, e.Name, e.ID)

	if _, err := r.etcdClient.Set(key, e.URL, endpointTTL); err != nil {
		return err
	}

	return nil
}

// RegisterLocation registers a provided location in vulcan.
func (r *Registry) RegisterLocation(l *Location) error {
	key := fmt.Sprintf(locationKey, l.Host, l.ID)

	pathKey := fmt.Sprintf("%v/path", key)
	if _, err := r.etcdClient.Set(pathKey, l.Path, 0); err != nil {
		return err
	}

	upstreamKey := fmt.Sprintf("%v/upstream", key)
	if _, err := r.etcdClient.Set(upstreamKey, l.Upstream, 0); err != nil {
		return err
	}

	options, err := json.Marshal(l.Options)
	if err != nil {
		return fmt.Errorf("failed to marshal %v: %v", l.Options, err)
	}

	optionsKey := fmt.Sprintf("%v/options", key)
	if _, err := r.etcdClient.Set(optionsKey, string(options), 0); err != nil {
		return err
	}

	for i, m := range l.Middlewares {
		// each middleware has a priority defining its order in the execution chain;
		// assign them priorities according to their positions in the list
		m.Priority = i

		middleware, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("failed to marshal %v: %v", m, err)
		}

		middlewareKey := fmt.Sprintf("%v/middlewares/%v/%v", key, m.Type, m.ID)
		if _, err := r.etcdClient.Set(middlewareKey, string(middleware), 0); err != nil {
			return err
		}
	}

	return nil
}

func (r *Registry) RegisterBackend(e *Endpoint) error {
	bk := fmt.Sprintf(backendPath+"/backend", e.Name)
	bkSpec, err := e.BackendSpec()
	if err != nil {
		return err
	}
	_, err = r.etcdClient.Set(bk, bkSpec, 0)
	return err
}

func (r *Registry) RegisterServer(e *Endpoint) error {
	if err := r.RegisterBackend(e); err != nil {
		return err
	}
	sSpec, err := e.ServerSpec()
	if err != nil {
		return err
	}
	sk := fmt.Sprintf(serverPath, e.Name, e.ID)
	if _, err := r.etcdClient.Set(sk, sSpec, endpointTTL); err != nil {
		return err
	}

	return nil
}

func (r *Registry) RegisterFrontend(l *Location) error {
	fk := fmt.Sprintf(frontendPath, l.Host, l.ID)

	spec, err := l.Spec()
	if err != nil {
		return err
	}

	if _, err := r.etcdClient.Set(fk+"/frontend", spec, 0); err != nil {
		return err
	}

	for i, m := range l.Middlewares {
		// each middleware has a priority defining its order in the execution chain;
		// assign them priorities according to their positions in the list
		m.Priority = i

		middleware, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("failed to marshal %v: %v", m, err)
		}

		middlewareKey := fmt.Sprintf("%v/middlewares/%v", fk, m.ID)
		if _, err := r.etcdClient.Set(middlewareKey, string(middleware), 0); err != nil {
			return err
		}
	}

	return nil
}
