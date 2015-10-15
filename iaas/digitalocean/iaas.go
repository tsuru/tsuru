// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package digitalocean

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/digitalocean/godo"
	"github.com/tsuru/tsuru/iaas"
	"golang.org/x/oauth2"
)

func init() {
	iaas.RegisterIaasProvider("digitalocean", newDigitalOceanIaas)
}

type digitalOceanIaas struct {
	base   iaas.UserDataIaaS
	client *godo.Client
}

func newDigitalOceanIaas(name string) iaas.IaaS {
	baseIaas := iaas.UserDataIaaS{NamedIaaS: iaas.NamedIaaS{BaseIaaSName: "digitalocean", IaaSName: name}}
	return &digitalOceanIaas{base: baseIaas}
}

func (i *digitalOceanIaas) Auth() error {
	u, _ := i.base.GetConfigString("url")
	token, err := i.base.GetConfigString("token")
	if err != nil {
		return err
	}
	client := http.Client{
		Transport: &oauth2.Transport{
			Source: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
		},
	}
	i.client = godo.NewClient(&client)
	if u != "" {
		i.client.BaseURL, err = url.Parse(u)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *digitalOceanIaas) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	i.Auth()
	image := godo.DropletCreateImage{Slug: params["image"]}
	userData, err := i.base.ReadUserData()
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
	droplet, _, err := i.client.Droplets.Create(createRequest)
	if err != nil {
		return nil, err
	}
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

func (i *digitalOceanIaas) waitNetworkCreated(droplet *godo.Droplet) (*godo.Droplet, error) {
	completed := false
	maxTry := 2
	for !completed && maxTry != 0 {
		var err error
		droplet, _, err = i.client.Droplets.Get(droplet.ID)
		if err != nil {
			return nil, err
		}
		if len(droplet.Networks.V4) == 0 {
			maxTry -= 1
			time.Sleep(5 * time.Second)
			continue
		}
		completed = true
	}
	if !completed {
		return nil, fmt.Errorf("Machine created but without network")
	}
	return droplet, nil
}

func (i *digitalOceanIaas) DeleteMachine(m *iaas.Machine) error {
	i.Auth()
	machineId, _ := strconv.Atoi(m.Id)
	resp, err := i.client.Droplets.Delete(machineId)
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("Failed to delete machine")
	}
	return nil
}

func (i *digitalOceanIaas) Describe() string {
	return `DigitalOcean IaaS required params:
  name=<name>                Your machine name
  region=<region>            Chosen region from DigitalOcean
  size=<size>                Your machine size
  image=<image>              The image ID of a public or private image

Further params will also be sent to digitalocean's deployVirtualMachine command.
`
}
