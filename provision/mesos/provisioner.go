// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/gambol99/go-marathon"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2/bson"
)

const (
	provisionerName = "mesos"
)

var errNotImplemented = errors.New("not implemented")

type mesosProvisioner struct{}

func init() {
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return &mesosProvisioner{}, nil
	})
}

func (p *mesosProvisioner) GetName() string {
	return provisionerName
}

func (p *mesosProvisioner) Provision(provision.App) error {
	return nil
}

func (p *mesosProvisioner) Destroy(provision.App) error {
	return errNotImplemented
}

func (p *mesosProvisioner) AddUnits(provision.App, uint, string, io.Writer) error {
	return errNotImplemented
}

func (p *mesosProvisioner) RemoveUnits(provision.App, uint, string, io.Writer) error {
	return errNotImplemented
}

func (p *mesosProvisioner) SetUnitStatus(provision.Unit, provision.Status) error {
	return errNotImplemented
}

func (p *mesosProvisioner) Restart(provision.App, string, io.Writer) error {
	return errNotImplemented
}

func (p *mesosProvisioner) Start(provision.App, string) error {
	return errNotImplemented
}

func (p *mesosProvisioner) Stop(provision.App, string) error {
	return errNotImplemented
}

func (p *mesosProvisioner) Units(app provision.App) ([]provision.Unit, error) {
	return nil, nil
}

func (p *mesosProvisioner) RoutableAddresses(app provision.App) ([]url.URL, error) {
	return nil, errNotImplemented
}

func (p *mesosProvisioner) RegisterUnit(a provision.App, unitId string, customData map[string]interface{}) error {
	return errNotImplemented
}

func (p *mesosProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	coll, err := nodeAddrCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var data mesosNodeWrapper
	err = coll.FindId(uniqueDocumentID).One(&data)
	if err != nil {
		return []provision.Node{}, nil
	}
	if len(addressFilter) > 0 {
		for _, addr := range addressFilter {
			if addr == data.Address() {
				return []provision.Node{&data}, nil
			}
		}
		return []provision.Node{}, nil
	}
	return []provision.Node{&data}, nil
}

func (p *mesosProvisioner) GetNode(address string) (provision.Node, error) {
	nodes, err := p.ListNodes(nil)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		if address == n.Address() {
			return n, nil
		}
	}
	return nil, provision.ErrNodeNotFound
}

func (p *mesosProvisioner) AddNode(opts provision.AddNodeOptions) error {
	coll, err := nodeAddrCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	addrs := []string{opts.Address}
	_, err = coll.UpsertId(uniqueDocumentID, bson.M{"$set": bson.M{"addresses": addrs}})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (p *mesosProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	return errNotImplemented
}

func (p *mesosProvisioner) UpdateNode(provision.UpdateNodeOptions) error {
	return errNotImplemented
}

func (p *mesosProvisioner) ArchiveDeploy(app provision.App, archiveURL string, evt *event.Event) (imgID string, err error) {
	return "", errNotImplemented
}

func (p *mesosProvisioner) ImageDeploy(a provision.App, imgID string, evt *event.Event) (string, error) {
	hosts, err := p.ListNodes(nil)
	if err != nil {
		return "", err
	}
	marathonURL := hosts[0].Address()
	config := marathon.NewDefaultConfig()
	config.URL = marathonURL
	client, err := marathon.NewClient(config)
	if err != nil {
		return "", err
	}
	application := marathon.NewDockerApplication().Name(a.GetName()).CPU(0.1).Memory(64).Count(1)
	if !strings.Contains(imgID, ":") {
		imgID = fmt.Sprintf("%s:latest", imgID)
	}
	application.Container.Docker.Container(imgID)
	_, err = client.CreateApplication(application)
	return "", err
}
