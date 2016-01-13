package vulcan

import (
	"encoding/json"
	"fmt"

	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/coreos/go-etcd/etcd"
)

const (
	etcdMachine   = "http://127.0.0.1:4001"
	frontendKey   = "%s/frontends/%s.%s/frontend"
	middlewareKey = "%s/frontends/%s.%s/middlewares/%s"
	backendKey    = "%s/backends/%s/backend"
	serverKey     = "%s/backends/%s/servers/%s"
)

type Client struct {
	Key  string
	etcd *etcd.Client
}

func NewClient(key string) *Client {
	etcd := etcd.NewClient([]string{etcdMachine})

	return &Client{Key: key, etcd: etcd}
}

func (c *Client) CreateServer(endpoint *Endpoint, ttl uint64) error {
	key := fmt.Sprintf(serverKey, c.Key, endpoint.Name, endpoint.ID)
	server, err := endpoint.ServerSpec()
	if err != nil {
		return nil
	}

	_, err = c.etcd.Create(key, server, ttl)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) UpdateServer(endpoint *Endpoint, ttl uint64) error {
	key := fmt.Sprintf(serverKey, c.Key, endpoint.Name, endpoint.ID)
	server, err := endpoint.ServerSpec()
	if err != nil {
		return nil
	}

	_, err = c.etcd.CompareAndSwap(key, server, ttl, server, 0)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) UpsertServer(endpoint *Endpoint, ttl uint64) error {
	key := fmt.Sprintf(serverKey, c.Key, endpoint.Name, endpoint.ID)
	server, err := endpoint.ServerSpec()
	if err != nil {
		return nil
	}

	_, err = c.etcd.Set(key, server, ttl)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RegisterBackend(endpoint *Endpoint) error {
	key := fmt.Sprintf(backendKey, c.Key, endpoint.Name)
	backend, err := endpoint.BackendSpec()
	if err != nil {
		return err
	}

	_, err = c.etcd.Set(key, backend, 0)
	if err != nil {
		return err
	}

	return err
}

func (c *Client) RegisterFrontend(location *Location) error {
	key := fmt.Sprintf(frontendKey, c.Key, location.Host, location.ID)
	frontend, err := location.Spec()
	if err != nil {
		return err
	}

	_, err = c.etcd.Set(key, frontend, 0)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RegisterMiddleware(location *Location) error {
	for i, m := range location.Middlewares {
		m.Priority = i

		key := fmt.Sprintf(middlewareKey, c.Key, location.Host, location.ID, m.ID)
		middleware, err := json.Marshal(m)
		if err != nil {
			return err
		}

		_, err = c.etcd.Set(key, string(middleware), 0)
		if err != nil {
			return err
		}
	}

	return nil
}
