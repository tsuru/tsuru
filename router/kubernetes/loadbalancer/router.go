package loadbalancer

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
)

var (
	_ router.Router     = &loadbalancerRouter{}
	_ router.OptsRouter = &loadbalancerRouter{}
	_ router.InfoRouter = &loadbalancerRouter{}

	errNotImplementedYet = errors.New("Not implemented yet")
)

const (
	// defaultLBPort is the default exposed port to the LB
	defaultLBPort = 80

	// exposeAllPortsOpt is the flag used to expose all ports in the LB
	exposeAllPortsOpt = "expose-all-ports"
	exposePortOpt     = "exposed-port"
)

const routerType = "kubernetes_loadbalancer"

func init() {
	router.Register(routerType, createRouter)
}

func createRouter(routerName string, config router.ConfigGetter) (router.Router, error) {
	return &loadbalancerRouter{
		routerName: routerName,
	}, nil
}

type loadbalancerRouter struct {
	routerName string
}

func (r *loadbalancerRouter) AddBackend(app router.App) (err error) {
	return r.AddBackendOpts(app, nil)
}

func (r *loadbalancerRouter) AddBackendOpts(app router.App, opts map[string]string) error {
	return r.syncLB(app, opts)
}

func (r *loadbalancerRouter) UpdateBackendOpts(app router.App, opts map[string]string) error {
	return r.syncLB(app, opts)
}

func (r *loadbalancerRouter) RemoveBackend(name string) (err error) {
	return errNotImplementedYet
}

func (r *loadbalancerRouter) AddRoutes(name string, addresses []*url.URL) (err error) {
	return errNotImplementedYet
}

func (r *loadbalancerRouter) RemoveRoutes(name string, addresses []*url.URL) (err error) {
	return errNotImplementedYet
}

func (r *loadbalancerRouter) Addr(name string) (addr string, err error) {
	return "", errNotImplementedYet
}

func (r *loadbalancerRouter) Routes(name string) (result []*url.URL, err error) {
	return nil, errNotImplementedYet
}

func (r *loadbalancerRouter) Swap(backend1 string, backend2 string, cnameOnly bool) (err error) {
	return errNotImplementedYet
}

func (r *loadbalancerRouter) GetName() string {
	return r.routerName
}

func (r *loadbalancerRouter) GetInfo() (map[string]string, error) {
	return map[string]string{
		exposeAllPortsOpt: "Expose all ports used by application in the Load Balancer. Defaults to false.",
		exposePortOpt:     "Port to be exposed by the Load Balancer. Defaults to 80.",
	}, nil
}
func (r *loadbalancerRouter) syncLB(app router.App, opts map[string]string) error {
	provisioner, err := provision.Get("kubernetes")
	if err != nil {
		return err
	}
	fmt.Println("provisioner ***", provisioner, err)
	//pool := app.GetPool()
	// TODO: discover the cluster
	return errNotImplementedYet
}
