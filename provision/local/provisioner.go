package local

import "github.com/globocom/tsuru/provision"

type LocalProvisioner struct{}

func (*LocalProvisioner) Provision(app provision.App) error {
	return nil
}
