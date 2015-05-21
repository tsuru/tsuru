package digitalocean

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"code.google.com/p/goauth2/oauth"
	"github.com/digitalocean/godo"

	"github.com/tsuru/tsuru/iaas"
)

func init() {
	iaas.RegisterIaasProvider("digitalocean", NewDigitalOceanIaas())
}

type DigitalOceanIaas struct {
	base   iaas.UserDataIaaS
	client *godo.Client
}

func NewDigitalOceanIaas() *DigitalOceanIaas {
	return &DigitalOceanIaas{base: iaas.UserDataIaaS{NamedIaaS: iaas.NamedIaaS{BaseIaaSName: "digitalocean"}}}
}

func (i *DigitalOceanIaas) Auth() error {
	u, err := i.base.GetConfigString("url")
	token, err := i.base.GetConfigString("token")
	if err != nil {
		return err
	}
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: token},
	}
	i.client = godo.NewClient(t.Client())
	i.client.BaseURL, err = url.Parse(u)
	if err != nil {
		return err
	}
	return nil
}

func (i *DigitalOceanIaas) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	i.Auth()
	image := godo.DropletCreateImage{Slug: params["image"]}
	userData, err := i.base.ReadUserDataAsString()
	if err != nil {
		return nil, err
	}
	createRequest := &godo.DropletCreateRequest{
		Name:     params["name"],
		Region:   params["region"],
		Size:     params["size"],
		Image:    image,
		UserData: userData,
	}
	newDroplet, _, err := i.client.Droplets.Create(createRequest)
	if err != nil {
		return nil, err
	}
	droplet := newDroplet.Droplet
	droplet, err = i.waitNetworkCreated(droplet)
	if err != nil {
		return nil, err
	}
	m := &iaas.Machine{
		Address: droplet.Networks.V4[0].IPAddress,
		Id:      strconv.Itoa(droplet.ID),
		Status:  droplet.Status,
	}
	return m, nil
}

func (i *DigitalOceanIaas) waitNetworkCreated(droplet *godo.Droplet) (*godo.Droplet, error) {
	completed := false
	maxTry := 2
	var d *godo.DropletRoot
	for !completed && maxTry != 0 {
		var err error
		d, _, err = i.client.Droplets.Get(droplet.ID)
		if err != nil {
			return nil, err
		}
		if len(d.Droplet.Networks.V4) == 0 {
			maxTry -= 1
			time.Sleep(5 * time.Second)
			continue
		}
		completed = true
	}
	if !completed {
		return nil, fmt.Errorf("Machine created but without network")
	}
	return d.Droplet, nil
}

func (i *DigitalOceanIaas) DeleteMachine(m *iaas.Machine) error {
	i.Auth()
	machine_id, _ := strconv.Atoi(m.Id)
	resp, err := i.client.Droplets.Delete(machine_id)
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("Failed to delete machine")
	}
	return nil
}

func (i *DigitalOceanIaas) Describe() string {
	return `DigitalOcean IaaS required params:
  name=<name>                Your machine name
  region=<region>            Chosen region from DigitalOcean
  size=<size>                Your machine size
  image=<image>              The image ID of a public or private image

Further params will also be sent to digitalocean's deployVirtualMachine command.
`
}
