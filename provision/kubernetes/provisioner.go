// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2/bson"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/restclient"
	client "k8s.io/kubernetes/pkg/client/unversioned"
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
	return nil
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

func (p *kubernetesProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	coll, err := nodeAddrCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var data kubernetesNodeWrapper
	err = coll.FindId(uniqueDocumentID).One(&data)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	return []provision.Node{&data}, nil
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
	token, err := config.GetString("kubernetes:token")
	if err != nil {
		return err
	}
	_, err = client.New(&restclient.Config{
		Host:        opts.Address,
		Insecure:    true,
		BearerToken: token,
	})
	if err != nil {
		return err
	}
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
	hosts, err := p.ListNodes(nil)
	if err != nil {
		return "", err
	}
	token, err := config.GetString("kubernetes:token")
	if err != nil {
		return "", err
	}
	client, err := client.New(&restclient.Config{
		Host:        hosts[0].Address(),
		Insecure:    true,
		BearerToken: token,
	})
	if err != nil {
		return "", err
	}
	if !strings.Contains(imgID, ":") {
		imgID = fmt.Sprintf("%s:latest", imgID)
	}
	deployment := extensions.Deployment{
		Spec: extensions.DeploymentSpec{
			Replicas: 1,
			Template: api.PodTemplateSpec{
				Spec: api.PodSpec{
					Containers: []api.Container{
						{Name: a.GetName(), Image: imgID},
					},
				},
			},
		},
	}
	_, err = client.Deployments("").Create(&deployment)
	return "", err
}
