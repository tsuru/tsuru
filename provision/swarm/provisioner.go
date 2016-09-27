// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"
	"io"
	"net"
	"net/url"

	"github.com/docker/engine-api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/event"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
)

const (
	provisionerName = "swarm"

	labelInternalPrefix = "tsuru-internal-"
	labelDockerAddr     = labelInternalPrefix + "docker-addr"
)

var errNotImplemented = errors.New("not implemented")

type swarmProvisioner struct{}

func init() {
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return &swarmProvisioner{}, nil
	})
}

func (p *swarmProvisioner) Initialize() error {
	var err error
	swarmConfig.swarmPort, err = config.GetInt("swarm:swarm-port")
	if err != nil {
		swarmConfig.swarmPort = 2377
	}
	caPath, _ := config.GetString("swarm:tls:root-path")
	if caPath != "" {
		swarmConfig.tlsConfig, err = readTLSConfig(caPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *swarmProvisioner) GetName() string {
	return provisionerName
}

func (p *swarmProvisioner) Provision(provision.App) error {
	return nil
}

func (p *swarmProvisioner) Destroy(provision.App) error {
	return errNotImplemented
}

func (p *swarmProvisioner) AddUnits(provision.App, uint, string, io.Writer) ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (p *swarmProvisioner) RemoveUnits(provision.App, uint, string, io.Writer) error {
	return errNotImplemented
}

func (p *swarmProvisioner) SetUnitStatus(provision.Unit, provision.Status) error {
	return errNotImplemented
}

func (p *swarmProvisioner) Restart(provision.App, string, io.Writer) error {
	return errNotImplemented
}

func (p *swarmProvisioner) Start(provision.App, string) error {
	return errNotImplemented
}

func (p *swarmProvisioner) Stop(provision.App, string) error {
	return errNotImplemented
}

func (p *swarmProvisioner) Units(app provision.App) ([]provision.Unit, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return nil, err
	}
	service, err := client.InspectService(app.GetName())
	if err != nil {
		if _, ok := err.(*docker.NoSuchService); ok {
			return nil, nil
		}
		return nil, errors.Wrap(err, "")
	}
	var pubPort uint32
	if len(service.Endpoint.Ports) > 0 {
		pubPort = service.Endpoint.Ports[0].PublishedPort
	}
	tasks, err := client.ListTasks(docker.ListTasksOptions{
		Filters: map[string][]string{
			"service": []string{app.GetName()},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	nodeMap := map[string]*swarm.Node{}
	units := make([]provision.Unit, len(tasks))
	for i, t := range tasks {
		if _, ok := nodeMap[t.NodeID]; !ok {
			var node *swarm.Node
			node, err = client.InspectNode(t.NodeID)
			if err != nil {
				return nil, errors.Wrap(err, "")
			}
			nodeMap[node.ID] = node
		}
		addr := nodeMap[t.NodeID].ManagerStatus.Addr
		host, _, _ := net.SplitHostPort(addr)
		units[i] = provision.Unit{
			ID:      t.Status.ContainerStatus.ContainerID,
			AppName: app.GetName(),
			// TODO(cezarsa): no process support for now, must add latter.
			ProcessName: "",
			Type:        app.GetPlatform(),
			Ip:          host,
			Status:      provision.StatusStarted,
			Address: &url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", host, pubPort),
			},
		}
	}
	return units, nil
}

func (p *swarmProvisioner) RoutableUnits(app provision.App) ([]provision.Unit, error) {
	// TODO(cezarsa): filter only routable units using process name
	return p.Units(app)
}

func (p *swarmProvisioner) RegisterUnit(provision.Unit, map[string]interface{}) error {
	return errNotImplemented
}

func (p *swarmProvisioner) SetNodeStatus(provision.NodeStatusData) error {
	return errNotImplemented
}

func (p *swarmProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		if errors.Cause(err) == errNoSwarmNode {
			return nil, nil
		}
		return nil, err
	}
	nodes, err := client.ListNodes(docker.ListNodesOptions{})
	if err != nil {
		return nil, err
	}
	var filterMap map[string]struct{}
	if len(addressFilter) > 0 {
		filterMap = map[string]struct{}{}
		for _, addr := range addressFilter {
			filterMap[tsuruNet.URLToHost(addr)] = struct{}{}
		}
	}
	nodeList := make([]provision.Node, 0, len(nodes))
	for i := range nodes {
		wrapped := &swarmNodeWrapper{Node: &nodes[i]}
		toAdd := true
		if filterMap != nil {
			_, toAdd = filterMap[tsuruNet.URLToHost(wrapped.Address())]
		}
		if toAdd {
			nodeList = append(nodeList, wrapped)
		}
	}
	return nodeList, nil
}

func (p *swarmProvisioner) GetNode(address string) (provision.Node, error) {
	nodes, err := p.ListNodes([]string{address})
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, provision.ErrNodeNotFound
	}
	return nodes[0], nil
}

func (p *swarmProvisioner) AddNode(opts provision.AddNodeOptions) error {
	existingClient, err := chooseDBSwarmNode()
	if err != nil && errors.Cause(err) != errNoSwarmNode {
		return err
	}
	newClient, err := newClient(opts.Address)
	if err != nil {
		return err
	}
	if existingClient == nil {
		err = initSwarm(newClient, opts.Address)
	} else {
		err = joinSwarm(existingClient, newClient, opts.Address)
	}
	if err != nil {
		return err
	}
	dockerInfo, err := newClient.Info()
	if err != nil {
		return errors.Wrap(err, "")
	}
	nodeData, err := newClient.InspectNode(dockerInfo.Swarm.NodeID)
	if err != nil {
		return errors.Wrap(err, "")
	}
	nodeData.Spec.Annotations.Labels = map[string]string{
		labelDockerAddr: opts.Address,
	}
	for k, v := range opts.Metadata {
		nodeData.Spec.Annotations.Labels[k] = v
	}
	err = newClient.UpdateNode(dockerInfo.Swarm.NodeID, docker.UpdateNodeOptions{
		Version:  nodeData.Version.Index,
		NodeSpec: nodeData.Spec,
	})
	if err != nil {
		return errors.Wrap(err, "")
	}
	return updateDBSwarmNodes(newClient)
}

func (p *swarmProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	node, err := p.GetNode(opts.Address)
	if err != nil {
		return err
	}
	client, err := chooseDBSwarmNode()
	if err != nil {
		return err
	}
	swarmNode := node.(*swarmNodeWrapper).Node
	if opts.Rebalance {
		swarmNode.Spec.Availability = swarm.NodeAvailabilityDrain
		err = client.UpdateNode(swarmNode.ID, docker.UpdateNodeOptions{
			NodeSpec: swarmNode.Spec,
			Version:  swarmNode.Version.Index,
		})
		if err != nil {
			return errors.Wrap(err, "")
		}
	}
	err = client.RemoveNode(docker.RemoveNodeOptions{
		ID:    swarmNode.ID,
		Force: true,
	})
	if err != nil {
		return errors.Wrap(err, "")
	}
	return updateDBSwarmNodes(client)
}

func (p *swarmProvisioner) UpdateNode(provision.UpdateNodeOptions) error {
	return errNotImplemented
}

func serviceSpecForApp(app provision.App, image string, baseSpec *swarm.ServiceSpec) (swarm.ServiceSpec, error) {
	var envs []string
	for _, envData := range app.Envs() {
		envs = append(envs, fmt.Sprintf("%s=%s", envData.Name, envData.Value))
	}
	host, _ := config.GetString("host")
	envs = append(envs, []string{
		fmt.Sprintf("%s=%s", "port", "8888"),
		fmt.Sprintf("%s=%s", "PORT", "8888"),
		fmt.Sprintf("%s=%s", "TSURU_HOST", host),
	}...)
	var unitCount uint64 = 1
	if baseSpec != nil {
		unitCount = *baseSpec.Mode.Replicated.Replicas
	}
	spec := swarm.ServiceSpec{
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: swarm.ContainerSpec{
				Image: image,
				Env:   envs,
			},
			RestartPolicy: &swarm.RestartPolicy{
				Condition: swarm.RestartPolicyConditionAny,
			},
			Placement: &swarm.Placement{
				Constraints: []string{
					fmt.Sprintf("node.labels.pool == %s", app.GetPool()),
				},
			},
		},
		EndpointSpec: &swarm.EndpointSpec{
			Mode: swarm.ResolutionModeVIP,
			Ports: []swarm.PortConfig{
				{TargetPort: 8888, PublishedPort: 0},
			},
		},
		Annotations: swarm.Annotations{
			Name: app.GetName(),
		},
		Mode: swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{
				Replicas: &unitCount,
			},
		},
	}
	return spec, nil
}

func (p *swarmProvisioner) ImageDeploy(app provision.App, image string, evt *event.Event) (string, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return "", err
	}
	srv, err := client.InspectService(app.GetName())
	if err != nil {
		if _, isNotFound := err.(*docker.NoSuchService); !isNotFound {
			return "", errors.Wrap(err, "")
		}
	}
	if srv == nil {
		var spec swarm.ServiceSpec
		spec, err = serviceSpecForApp(app, image, nil)
		if err != nil {
			return "", err
		}
		srv, err = client.CreateService(docker.CreateServiceOptions{
			ServiceSpec: spec,
		})
		if err != nil {
			return "", errors.Wrap(err, "")
		}
	} else {
		srv.Spec, err = serviceSpecForApp(app, image, &srv.Spec)
		if err != nil {
			return "", err
		}
		err = client.UpdateService(srv.ID, docker.UpdateServiceOptions{
			Version:     srv.Version.Index,
			ServiceSpec: srv.Spec,
		})
		if err != nil {
			return "", errors.Wrap(err, "")
		}
	}
	return image, nil
}
