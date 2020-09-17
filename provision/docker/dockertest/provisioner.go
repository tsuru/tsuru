// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import (
	"context"
	"io"
	"strings"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/clusterclient"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

type ContainerMoving struct {
	ContainerID string
	HostFrom    string
	HostTo      string
}

type FakeDockerProvisioner struct {
	containers      map[string][]container.Container
	containersMut   sync.Mutex
	queries         []bson.M
	storage         *cluster.MapStorage
	cluster         *cluster.Cluster
	pushes          []Push
	servers         []*testing.DockerServer
	pushErrors      chan error
	moveErrors      chan error
	preparedErrors  chan error
	preparedResults chan []container.Container
	movings         []ContainerMoving
	actionLimiter   provision.ActionLimiter
}

func NewFakeDockerProvisioner(servers ...string) (*FakeDockerProvisioner, error) {
	var err error
	p := FakeDockerProvisioner{
		storage:         &cluster.MapStorage{},
		pushErrors:      make(chan error, 10),
		moveErrors:      make(chan error, 10),
		preparedErrors:  make(chan error, 10),
		preparedResults: make(chan []container.Container, 10),
		containers:      make(map[string][]container.Container),
		actionLimiter:   &provision.LocalLimiter{},
	}
	nodes := make([]cluster.Node, len(servers))
	for i, server := range servers {
		nodes[i] = cluster.Node{Address: server}
	}
	p.cluster, err = cluster.New(nil, p.storage, "", nodes...)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func StartMultipleServersCluster() (*FakeDockerProvisioner, error) {
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		return nil, err
	}
	server2, err := testing.NewServer("localhost:0", nil, nil)
	if err != nil {
		return nil, err
	}
	otherURL := strings.Replace(server2.URL(), "127.0.0.1", "localhost", 1)
	p, err := NewFakeDockerProvisioner(server1.URL(), otherURL)
	if err != nil {
		return nil, err
	}
	p.servers = []*testing.DockerServer{server1, server2}
	return p, nil
}

func (p *FakeDockerProvisioner) ActionLimiter() provision.ActionLimiter {
	return p.actionLimiter
}

func (p *FakeDockerProvisioner) Destroy() {
	for _, server := range p.servers {
		server.Stop()
	}
	p.servers = nil
}

func (p *FakeDockerProvisioner) Servers() []*testing.DockerServer {
	return p.servers
}

func (p *FakeDockerProvisioner) GetName() string {
	return "fake"
}

func (p *FakeDockerProvisioner) FailPush(errs ...error) {
	for _, err := range errs {
		p.pushErrors <- err
	}
}

func (p *FakeDockerProvisioner) Cluster() *cluster.Cluster {
	return p.cluster
}

func (p *FakeDockerProvisioner) ClusterClient() provision.BuilderDockerClient {
	return &clusterclient.ClusterClient{
		Cluster:    p.Cluster(),
		Limiter:    p.ActionLimiter(),
		Collection: p.Collection,
	}
}

func (p *FakeDockerProvisioner) Collection() *storage.Collection {
	conn, err := db.Conn()
	if err != nil {
		panic(err)
	}
	return conn.Collection("fake_docker_provisioner")
}

func (p *FakeDockerProvisioner) PushImage(name, tag string) error {
	p.pushes = append(p.pushes, Push{Name: name, Tag: tag})
	select {
	case err := <-p.pushErrors:
		return err
	default:
	}
	return nil
}

type Push struct {
	Name string
	Tag  string
}

func (p *FakeDockerProvisioner) Pushes() []Push {
	return p.pushes
}

func (p *FakeDockerProvisioner) SetContainers(host string, containers []container.Container) {
	p.containersMut.Lock()
	defer p.containersMut.Unlock()
	dst := make([]container.Container, len(containers))
	for i, container := range containers {
		container.HostAddr = host
		dst[i] = container
	}
	p.containers[host] = dst
}

func (p *FakeDockerProvisioner) Containers(host string) []container.Container {
	p.containersMut.Lock()
	defer p.containersMut.Unlock()
	return p.containers[host]
}

func (p *FakeDockerProvisioner) AllContainers() []container.Container {
	p.containersMut.Lock()
	defer p.containersMut.Unlock()
	var result []container.Container
	for _, containers := range p.containers {
		result = append(result, containers...)
	}
	return result
}

func (p *FakeDockerProvisioner) Movings() []ContainerMoving {
	p.containersMut.Lock()
	defer p.containersMut.Unlock()
	return p.movings
}

func (p *FakeDockerProvisioner) FailMove(errs ...error) {
	for _, err := range errs {
		p.moveErrors <- err
	}
}

func (p *FakeDockerProvisioner) MoveOneContainer(ctx context.Context, cont container.Container, toHost string, errors chan error, wg *sync.WaitGroup, w io.Writer, locker container.AppLocker) container.Container {
	select {
	case err := <-p.moveErrors:
		errors <- err
		return container.Container{}
	default:
	}
	p.containersMut.Lock()
	defer p.containersMut.Unlock()
	cont, err := p.moveOneContainer(cont, toHost)
	if err != nil {
		errors <- err
	}
	return cont
}

func (p *FakeDockerProvisioner) moveOneContainer(cont container.Container, toHost string) (container.Container, error) {
	cont, index, err := p.findContainer(cont.ID)
	if err != nil {
		return cont, err
	}
	if cont.HostAddr == toHost {
		return cont, nil
	}
	if toHost == "" {
		for host := range p.containers {
			if host != cont.HostAddr {
				toHost = host
				break
			}
		}
	}
	originHost := cont.HostAddr
	moving := ContainerMoving{
		ContainerID: cont.ID,
		HostFrom:    originHost,
		HostTo:      toHost,
	}
	p.movings = append(p.movings, moving)
	if toHost == "" {
		cont.ID += "-recreated"
		p.containers[originHost][index] = cont
		return cont, nil
	}
	cont.HostAddr = toHost
	cont.ID += "-moved"
	last := len(p.containers[originHost]) - 1
	p.containers[originHost][index] = p.containers[originHost][last]
	p.containers[originHost] = p.containers[originHost][:last]
	p.containers[cont.HostAddr] = append(p.containers[cont.HostAddr], cont)
	return cont, nil
}

func (p *FakeDockerProvisioner) MoveContainers(fromHost, toHost string, w io.Writer) error {
	p.containersMut.Lock()
	defer p.containersMut.Unlock()
	containers, ok := p.containers[fromHost]
	if !ok {
		return errors.Errorf("host not found: %s", fromHost)
	}
	for _, container := range containers {
		_, err := p.moveOneContainer(container, toHost)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *FakeDockerProvisioner) HandleMoveErrors(errors chan error, w io.Writer) error {
	select {
	case err := <-errors:
		return err
	default:
		return nil
	}
}

func (p *FakeDockerProvisioner) GetContainer(id string) (*container.Container, error) {
	container, _, err := p.findContainer(id)
	return &container, err
}

// PrepareListResult prepares a result or a failure in the next ListContainers
// call. If err is not nil, it will prepare a failure. Otherwise it will
// prepare a valid result with the provided list of containers.
func (p *FakeDockerProvisioner) PrepareListResult(containers []container.Container, err error) {
	if err != nil {
		p.preparedErrors <- err
	} else if len(containers) > 0 {
		coll := p.Collection()
		defer coll.Close()
		for _, c := range containers {
			coll.Insert(c)
		}
	}
}

func (p *FakeDockerProvisioner) ListContainers(query bson.M) ([]container.Container, error) {
	p.queries = append(p.queries, query)
	select {
	case err := <-p.preparedErrors:
		return nil, err
	default:
	}
	coll := p.Collection()
	defer coll.Close()
	var insertedIDs []string
	defer func() {
		coll.RemoveAll(bson.M{"id": bson.M{"$in": insertedIDs}})
	}()
	for _, containers := range p.containers {
		for _, c := range containers {
			n, err := coll.Find(bson.M{"id": c.ID}).Count()
			if err != nil {
				return nil, err
			}
			if n > 0 {
				continue
			}
			insertedIDs = append(insertedIDs, c.ID)
			err = coll.Insert(c)
			if err != nil {
				return nil, err
			}
		}
	}
	var containers []container.Container
	err := coll.Find(query).All(&containers)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func (p *FakeDockerProvisioner) Queries() []bson.M {
	return p.queries
}

type StartContainersArgs struct {
	Endpoint  string
	App       provision.App
	Amount    map[string]int
	Image     string
	PullImage bool
}

// StartContainers starts the provided amount of containers in the provided
// endpoint.
//
// The amount is specified using a map of processes. The started containers
// will be both returned and stored internally.
func (p *FakeDockerProvisioner) StartContainers(args StartContainersArgs) ([]container.Container, error) {
	if args.PullImage {
		err := p.Cluster().PullImage(docker.PullImageOptions{Repository: args.Image}, dockercommon.RegistryAuthConfig(args.Image), args.Endpoint)
		if err != nil {
			return nil, err
		}
	}
	opts := docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: args.Image,
		},
	}
	hostAddr := net.URLToHost(args.Endpoint)
	createdContainers := make([]container.Container, 0, len(args.Amount))
	for processName, amount := range args.Amount {
		opts.Config.Cmd = []string{processName}
		for i := 0; i < amount; i++ {
			_, cont, err := p.Cluster().CreateContainer(opts, net.StreamInactivityTimeout, args.Endpoint)
			if err != nil {
				return nil, err
			}
			createdContainers = append(createdContainers, container.Container{
				Container: types.Container{
					ID:            cont.ID,
					AppName:       args.App.GetName(),
					ProcessName:   processName,
					Type:          args.App.GetPlatform(),
					Status:        provision.StatusCreated.String(),
					HostAddr:      hostAddr,
					Version:       "v1",
					Image:         args.Image,
					User:          "root",
					BuildingImage: args.Image,
					Routable:      true,
				},
			})
		}
	}
	p.containersMut.Lock()
	defer p.containersMut.Unlock()
	p.containers[hostAddr] = append(p.containers[hostAddr], createdContainers...)
	return createdContainers, nil
}

func (p *FakeDockerProvisioner) findContainer(id string) (container.Container, int, error) {
	for _, containers := range p.containers {
		for i, container := range containers {
			if container.ID == id {
				return container, i, nil
			}
		}
	}
	return container.Container{}, -1, &provision.UnitNotFoundError{ID: id}
}

func (p *FakeDockerProvisioner) DeleteContainer(id string) {
	p.containersMut.Lock()
	defer p.containersMut.Unlock()
	for h := range p.containers {
		for i := 0; i < len(p.containers[h]); i++ {
			if p.containers[h][i].ID == id {
				p.containers[h] = append(p.containers[h][:i], p.containers[h][i+1:]...)
				i--
			}
		}
	}
}
