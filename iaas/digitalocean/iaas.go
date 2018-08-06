// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package digitalocean

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/godo/util"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/net"
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
	client := *net.Dial15Full300Client
	client.Transport = &oauth2.Transport{
		Source: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
		Base:   net.Dial15Full300Client.Transport,
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
	userData, err := i.base.ReadUserData(params)
	if err != nil {
		return nil, err
	}
	var sshKeys []godo.DropletCreateSSHKey
	if rawSSHKeys, ok := params["ssh-keys"]; ok {
		for _, key := range strings.Split(rawSSHKeys, ",") {
			if keyID, atoiErr := strconv.Atoi(key); atoiErr == nil {
				sshKeys = append(sshKeys, godo.DropletCreateSSHKey{ID: keyID})
			} else {
				sshKeys = append(sshKeys, godo.DropletCreateSSHKey{Fingerprint: key})
			}
		}
	}
	privNetworking, _ := strconv.ParseBool(params["private-networking"])
	createRequest := &godo.DropletCreateRequest{
		Name:              params["name"],
		Region:            params["region"],
		Size:              params["size"],
		PrivateNetworking: privNetworking,
		Image:             image,
		SSHKeys:           sshKeys,
		UserData:          userData,
	}
	droplet, _, err := i.client.Droplets.Create(context.Background(), createRequest)
	if err != nil {
		return nil, err
	}
	droplet, err = i.waitNetworkCreated(droplet)
	if err != nil {
		return nil, err
	}
	ipAddress := droplet.Networks.V4[0].IPAddress
	if privNetworking && droplet.Networks.V4[0].Type != "private" {
		for _, network := range droplet.Networks.V4[1:] {
			if network.Type == "private" {
				ipAddress = network.IPAddress
				break
			}
		}
	}
	m := &iaas.Machine{
		Address: ipAddress,
		Id:      strconv.Itoa(droplet.ID),
		Status:  droplet.Status,
	}
	return m, nil
}

func (i *digitalOceanIaas) waitNetworkCreated(droplet *godo.Droplet) (*godo.Droplet, error) {
	rawTimeout, _ := i.base.GetConfigString("wait-timeout")
	timeout, _ := strconv.Atoi(rawTimeout)
	if timeout == 0 {
		timeout = 120
	}
	quit := make(chan struct{})
	errs := make(chan error, 1)
	droplets := make(chan *godo.Droplet, 1)
	go func() {
		for {
			select {
			case <-quit:
				return
			default:
				d, _, err := i.client.Droplets.Get(context.Background(), droplet.ID)
				if err != nil {
					errs <- err
					return
				}
				if len(d.Networks.V4) > 0 {
					droplets <- d
					return
				}
			}
		}
	}()
	select {
	case droplet = <-droplets:
		return droplet, nil
	case err := <-errs:
		return nil, err
	case <-time.After(time.Duration(timeout) * time.Second):
		return nil, errors.New("timed out waiting for machine network")
	}
}

func (i *digitalOceanIaas) DeleteMachine(m *iaas.Machine) error {
	i.Auth()
	machineId, _ := strconv.Atoi(m.Id)
	action, _, err := i.client.DropletActions.Shutdown(context.Background(), machineId)
	if err != nil {
		// PowerOff force the shutdown
		action, _, err = i.client.DropletActions.PowerOff(context.Background(), machineId)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	u, _ := i.base.GetConfigString("url")
	uri := fmt.Sprintf("%s/v2/actions/%d", strings.TrimRight(u, "/"), action.ID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err = util.WaitForActive(ctx, i.client, uri)
	if err != nil {
		return errors.WithStack(err)
	}
	resp, err := i.client.Droplets.Delete(context.Background(), machineId)
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.StatusCode != 204 {
		return errors.New("failed to delete machine")
	}
	return nil
}

func (i *digitalOceanIaas) Describe() string {
	return `DigitalOcean IaaS required params:
  name=<name>                Name of the droplet
  region=<region>            Chosen region from DigitalOcean (e.g.: nyc3)
  size=<size>                Your machine size (e.g.: 512mb)
  image=<image>              The image ID of a public or private image

There are also some optional parameters:

  private-networking=1/0     Whether to use private networking in this instance.
  ssh-keys=<keys>            Comma separated list of keys. The key can be identified
                             by its ID or fingerprint.
			     (e.g.: ssh_keys=5050,2032,07:b9:a1:65:1b,13 will result in
		             the key IDs 5050, 2032 and 13, along with the fingerprint
		             07:b9:a1:65:1b)
`
}
