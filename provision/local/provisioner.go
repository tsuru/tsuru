package local

import "github.com/globocom/tsuru/provision"

type LocalProvisioner struct{}

func (*LocalProvisioner) Provision(app provision.App) error {
	container := container{name: app.GetName()}
	err := container.create()
	if err != nil {
		return err
	}
	err = container.start()
	if err != nil {
		return err
	}
	return nil
}
