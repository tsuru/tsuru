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

func (*LocalProvisioner) Destroy(app provision.App) error {
	container := container{name: app.GetName()}
	err := container.stop()
	if err != nil {
		return err
	}
	err = container.destroy()
	if err != nil {
		return err
	}
	return nil
}

func (*LocalProvisioner) Addr(app provision.App) (string, error) {
	units := app.ProvisionUnits()
	return units[0].GetIp(), nil
}

func (*LocalProvisioner) AddUnits(app provision.App, units uint) ([]provision.Unit, error) {
	return []provision.Unit{}, nil
}

func (*LocalProvisioner) RemoveUnit(app provision.App, unitName string) error {
	return nil
}
