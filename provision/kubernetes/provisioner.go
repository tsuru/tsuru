// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"io"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2/bson"
)

const (
	provisionerName = "kubernetes"
)

var errNotImplemented = errors.New("not implemented")

type kubernetesProvisioner struct{}

func init() {
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return &kubernetesProvisioner{}, nil
	})
}

func (p *kubernetesProvisioner) GetName() string {
	return provisionerName
}

func (p *kubernetesProvisioner) Provision(provision.App) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) Destroy(provision.App) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) AddUnits(provision.App, uint, string, io.Writer) ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (p *kubernetesProvisioner) RemoveUnits(provision.App, uint, string, io.Writer) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) SetUnitStatus(provision.Unit, provision.Status) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) Restart(provision.App, string, io.Writer) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) Start(provision.App, string) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) Stop(provision.App, string) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) Units(app provision.App) ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (p *kubernetesProvisioner) RoutableUnits(app provision.App) ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (p *kubernetesProvisioner) RegisterUnit(unit provision.Unit, customData map[string]interface{}) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) SetNodeStatus(provision.NodeStatusData) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	return nil, errNotImplemented
}

func (p *kubernetesProvisioner) GetNode(address string) (provision.Node, error) {
	return nil, errNotImplemented
}

func (p *kubernetesProvisioner) AddNode(opts provision.AddNodeOptions) error {
	coll, err := nodeAddrCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	addrs := []string{opts.Address}
	_, err = coll.UpsertId(uniqueDocumentID, bson.M{"$set": bson.M{"addresses": addrs}})
	if err != nil {
		return errors.Wrap(err, "")
	}
	return nil
}

func (p *kubernetesProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) UpdateNode(provision.UpdateNodeOptions) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) ArchiveDeploy(app provision.App, archiveURL string, evt *event.Event) (imgID string, err error) {
	return "", errNotImplemented
}

func (p *kubernetesProvisioner) ImageDeploy(a provision.App, imgID string, evt *event.Event) (string, error) {
	return "", errNotImplemented
}
