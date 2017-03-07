// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/set"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
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
	client, err := getClusterClient()
	if err != nil {
		if err == errNoCluster {
			return nil, nil
		}
		return nil, err
	}
	nodeList, err := client.Core().Nodes().List(v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var addressSet set.Set
	if len(addressFilter) > 0 {
		addressSet = set.FromSlice(addressFilter)
	}
	var nodes []provision.Node
	for i := range nodeList.Items {
		n := &kubernetesNodeWrapper{
			node: &nodeList.Items[i],
			prov: p,
		}
		if addressSet == nil || addressSet.Includes(n.Address()) {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

func (p *kubernetesProvisioner) GetNode(address string) (provision.Node, error) {
	client, err := getClusterClient()
	if err != nil {
		return nil, err
	}
	node, err := p.findNodeByAddress(client, address)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (p *kubernetesProvisioner) AddNode(opts provision.AddNodeOptions) error {
	isCluster, _ := strconv.ParseBool(opts.Metadata["cluster"])
	if isCluster {
		return addClusterNode(opts)
	}
	// TODO(cezarsa): Start kubelet, kube-proxy and add labels
	return errNotImplemented
}

func (p *kubernetesProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	return errNotImplemented
}

func (p *kubernetesProvisioner) NodeForNodeData(nodeData provision.NodeStatusData) (provision.Node, error) {
	return provision.FindNodeByAddrs(p, nodeData.Addrs)
}

func (p *kubernetesProvisioner) findNodeByAddress(client kubernetes.Interface, address string) (*kubernetesNodeWrapper, error) {
	nodeList, err := client.Core().Nodes().List(v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range nodeList.Items {
		nodeWrapper := &kubernetesNodeWrapper{node: &nodeList.Items[i], prov: p}
		if address == nodeWrapper.Address() {
			return nodeWrapper, nil
		}
	}
	return nil, provision.ErrNodeNotFound
}

func (p *kubernetesProvisioner) UpdateNode(opts provision.UpdateNodeOptions) error {
	client, err := getClusterClient()
	if err != nil {
		return err
	}
	nodeWrapper, err := p.findNodeByAddress(client, opts.Address)
	if err != nil {
		return err
	}
	node := nodeWrapper.node
	if opts.Disable {
		node.Spec.Unschedulable = true
	} else if opts.Enable {
		node.Spec.Unschedulable = false
	}
	for k, v := range opts.Metadata {
		if v == "" {
			delete(node.Labels, k)
		} else {
			node.Labels[k] = v
		}
	}
	_, err = client.Core().Nodes().Update(node)
	return err
}

func deploymentNameForApp(a provision.App, process string) string {
	return fmt.Sprintf("%s-%s", a.GetName(), process)
}

func (p *kubernetesProvisioner) ImageDeploy(a provision.App, imgID string, evt *event.Event) (string, error) {
	client, err := getClusterClient()
	if err != nil {
		return "", err
	}
	if !strings.Contains(imgID, ":") {
		imgID = fmt.Sprintf("%s:latest", imgID)
	}
	routerName, err := a.GetRouterName()
	if err != nil {
		return "", errors.WithStack(err)
	}
	routerType, _, err := router.Type(routerName)
	if err != nil {
		return "", errors.WithStack(err)
	}
	replicas := int32(1)
	deployment := v1beta1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name: deploymentNameForApp(a, ""),
		},
		Spec: v1beta1.DeploymentSpec{
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"tsuru.pod":         strconv.FormatBool(true),
						"tsuru.app.name":    a.GetName(),
						"tsuru.node.pool":   a.GetPool(),
						"tsuru.router.name": routerName,
						"tsuru.router.type": routerType,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  a.GetName(),
							Image: imgID,
						},
					},
				},
			},
		},
	}
	// client.De
	_, err = client.Extensions().Deployments("default").Create(&deployment)
	return "", err
}
