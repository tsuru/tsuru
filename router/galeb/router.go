package galeb

import "github.com/tsuru/tsuru/router"

const routerName = "galeb"

type galebRouter struct{}

func init() {
	router.Register(routerName, &galebRouter{})
}

func (r *galebRouter) AddBackend(name string) error {
	return nil
}

func (r *galebRouter) RemoveBackend(name string) error {
	return nil
}

func (r *galebRouter) AddRoute(name, address string) error {
	return nil
}

func (r *galebRouter) RemoveRoute(name, address string) error {
	return nil
}

func (r *galebRouter) SetCName(cname, name string) error {
	return nil
}

func (r *galebRouter) UnsetCName(cname, name string) error {
	return nil
}

func (r *galebRouter) Addr(name string) (string, error) {
	return "", nil
}

func (r *galebRouter) Swap(string, string) error {
	return nil
}

func (r *galebRouter) Routes(name string) ([]string, error) {
	return nil, nil
}
