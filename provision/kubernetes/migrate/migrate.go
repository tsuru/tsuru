package migrate

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	tsuruerrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/kubernetes"
	"github.com/tsuru/tsuru/provision/pool"
)

// MigrateAppsCRDs creates the necessary CRDs for every application
// on a Kubernetes cluster. This is done by re-provisioning the App
// on the cluster.
func MigrateAppsCRDs() error {
	prov := kubernetes.GetProvisioner()
	pools, err := pool.ListAllPools()
	if err != nil {
		return errors.Wrap(err, "failed to list pools")
	}
	var kubePools []string
	for _, p := range pools {
		if p.Provisioner == prov.GetName() {
			kubePools = append(kubePools, p.Name)
		}
	}
	apps, err := app.List(&app.Filter{Pools: kubePools})
	if err != nil {
		return errors.Wrap(err, "failed to list apps")
	}
	multiErr := tsuruerrors.NewMultiError()
	for _, a := range apps {
		errProv := prov.Provision(&a)
		if errProv != nil {
			multiErr.Add(errProv)
		}
	}
	return multiErr.ToError()
}
