package digitalocean

import (
	"strconv"

	"code.google.com/p/goauth2/oauth"
	"github.com/tarsisazevedo/godo"

	"github.com/tsuru/tsuru/iaas"
)

type DigitalOceanIaas struct {
	base   iaas.UserDataIaaS
	client *godo.Client
}

func init() {
	iaas.RegisterIaasProvider("digitalocean", NewDigitalOceanIaaS())
}

func NewDigitalOceanIaas() *DigitalOceanIaas {
	return &DigitalOceanIaas{base: iaas.UserDataIaaS{NamedIaaS: iaas.NamedIaaS{BaseIaaSName: "digitalocean"}}}
}

func (i *DigitalOceanIaas) Auth() error {
	url, err := i.base.GetConfigString("url")
	token, err := i.base.GetConfigString("token")
	if err != nil {
		return err
	}
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: token},
	}
	i.client = godo.NewClient(t.Client(), url)
	return nil
}

func (i *DigitalOceanIaas) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	i.Auth()
	createRequest := &godo.DropletCreateRequest{
		Name:   params["name"],
		Region: params["region"],
		Size:   params["size"],
		Image:  params["image"],
	}
	newDroplet, _, err := i.client.Droplets.Create(createRequest)
	if err != nil {
		return nil, err
	}
	droplet := newDroplet.Droplet

	m := &iaas.Machine{
		Address: droplet.Networks.V4[0].IPAddress,
		Id:      strconv.Itoa(droplet.ID),
		Status:  droplet.Status,
	}
	return m, nil
}

func (i *DigitalOceanIaas) DeleteMachine(m *Machine) error {
	return nil
}
