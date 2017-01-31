// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"io"
	"net/url"
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

func (p *kubernetesProvisioner) AddUnits(provision.App, uint, string, io.Writer) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) RemoveUnits(provision.App, uint, string, io.Writer) error {
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
	return nil, nil
}

func (p *kubernetesProvisioner) RoutableAddresses(app provision.App) ([]url.URL, error) {
	return nil, errNotImplemented
}

func (p *kubernetesProvisioner) RegisterUnit(a provision.App, unitId string, customData map[string]interface{}) error {
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

func (p *kubernetesProvisioner) GetNode(address string) (provision.Node, error) {
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
		return errors.WithStack(err)
	}
	return nil
}

func (p *kubernetesProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	coll, err := nodeAddrCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.RemoveId(uniqueDocumentID)
	return err
}

func (p *kubernetesProvisioner) NodeForNodeData(nodeData provision.NodeStatusData) (provision.Node, error) {
	return provision.FindNodeByAddrs(p, nodeData.Addrs)
}

func (p *kubernetesProvisioner) UpdateNode(provision.UpdateNodeOptions) error {
	return nil
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
		ObjectMeta: api.ObjectMeta{
			Name: a.GetName(),
		},
		Spec: extensions.DeploymentSpec{
			Replicas: 1,
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Name: a.GetName(),
					Labels: map[string]string{
						"name": a.GetName(),
					},
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						{Name: a.GetName(), Image: imgID},
					},
				},
			},
		},
	}
	_, err = client.Deployments("default").Create(&deployment)
	return "", err
}
