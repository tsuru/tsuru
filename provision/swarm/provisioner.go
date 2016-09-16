package swarm

import (
	"io"

	"github.com/tsuru/tsuru/provision"
)

const provisionerName = "swarm"

type swarmProvisioner struct{}

func init() {
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return &swarmProvisioner{}, nil
	})
}

func (p *swarmProvisioner) GetName() string {
	return provisionerName
}

func (p *swarmProvisioner) Provision(provision.App) error {
	return nil
}

func (p *swarmProvisioner) Destroy(provision.App) error {
	return nil
}

func (p *swarmProvisioner) AddUnits(provision.App, uint, string, io.Writer) ([]provision.Unit, error) {
	return nil, nil
}

func (p *swarmProvisioner) RemoveUnits(provision.App, uint, string, io.Writer) error {
	return nil
}

func (p *swarmProvisioner) SetUnitStatus(provision.Unit, provision.Status) error {
	return nil
}

func (p *swarmProvisioner) Restart(provision.App, string, io.Writer) error {
	return nil
}

func (p *swarmProvisioner) Start(provision.App, string) error {
	return nil
}

func (p *swarmProvisioner) Stop(provision.App, string) error {
	return nil
}

func (p *swarmProvisioner) Units(provision.App) ([]provision.Unit, error) {
	return nil, nil
}

func (p *swarmProvisioner) RoutableUnits(provision.App) ([]provision.Unit, error) {
	return nil, nil
}

func (p *swarmProvisioner) RegisterUnit(provision.Unit, map[string]interface{}) error {
	return nil
}
