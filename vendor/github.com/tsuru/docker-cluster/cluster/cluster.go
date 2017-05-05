// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cluster provides types and functions for management of Docker
// clusters, scheduling container operations among hosts running Docker
// (nodes).
package cluster

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/docker-cluster/log"
)

var (
	errStorageMandatory = errors.New("Storage parameter is mandatory")
	errHealerInProgress = errors.New("Healer already running")

	defaultDialTimeout = 10 * time.Second
	defaultTimeout     = 5 * time.Minute
	shortDialTimeout   = 5 * time.Second
	shortTimeout       = 1 * time.Minute

	timeout10Dialer = &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
)

type node struct {
	*docker.Client
	addr string
}

func (n *node) setPersistentClient() {
	n.HTTPClient = clientWithTimeout(defaultDialTimeout, 0, n.TLSConfig)
}

// ContainerStorage provides methods to store and retrieve information about
// the relation between the node and the container. It can be easily
// represented as a key-value storage.
//
// The relevant information is: in which host the given container is running?
type ContainerStorage interface {
	StoreContainer(container, host string) error
	RetrieveContainer(container string) (host string, err error)
	RemoveContainer(container string) error
	RetrieveContainers() ([]Container, error)
}

// ImageStorage works like ContainerStorage, but stores information about
// images and hosts.
type ImageStorage interface {
	StoreImage(repo, id, host string) error
	RetrieveImage(repo string) (Image, error)
	RemoveImage(repo, id, host string) error
	RetrieveImages() ([]Image, error)
	SetImageDigest(repo, digest string) error
}

type NodeStorage interface {
	StoreNode(node Node) error
	RetrieveNodesByMetadata(metadata map[string]string) ([]Node, error)
	RetrieveNodes() ([]Node, error)
	RetrieveNode(address string) (Node, error)
	UpdateNode(node Node) error
	RemoveNode(address string) error
	RemoveNodes(addresses []string) error
	LockNodeForHealing(address string, isFailure bool, timeout time.Duration) (bool, error)
	ExtendNodeLock(address string, timeout time.Duration) error
	UnlockNode(address string) error
}

type Storage interface {
	ContainerStorage
	ImageStorage
	NodeStorage
}

type HookEvent int

const (
	HookEventBeforeContainerCreate = iota
	HookEventBeforeNodeRegister
	HookEventBeforeNodeUnregister
)

type Hook interface {
	RunClusterHook(evt HookEvent, node *Node) error
}

// Cluster is the basic type of the package. It manages internal nodes, and
// provide methods for interaction with those nodes, like CreateContainer,
// which creates a container in one node of the cluster.
type Cluster struct {
	Healer         Healer
	scheduler      Scheduler
	stor           Storage
	monitoringDone chan bool
	dryServer      *testing.DockerServer
	hooks          map[HookEvent][]Hook
	tlsConfig      *tls.Config
}

type DockerNodeError struct {
	node node
	cmd  string
	err  error
}

func (n DockerNodeError) Error() string {
	if n.cmd == "" {
		return fmt.Sprintf("error in docker node %q: %s", n.node.addr, n.err.Error())
	}
	return fmt.Sprintf("error in docker node %q running command %q: %s", n.node.addr, n.cmd, n.err.Error())
}

func (n DockerNodeError) BaseError() error {
	return n.err
}

func wrapError(n node, err error) error {
	if err != nil {
		return DockerNodeError{node: n, err: err}
	}
	return nil
}

func wrapErrorWithCmd(n node, err error, cmd string) error {
	if err != nil {
		return DockerNodeError{node: n, err: err, cmd: cmd}
	}
	return nil
}

// New creates a new Cluster, initially composed by the given nodes.
//
// The scheduler parameter defines the scheduling strategy. It defaults
// to round robin if nil.
// The storage parameter is the storage the cluster instance will use.
func New(scheduler Scheduler, storage Storage, caPath string, nodes ...Node) (*Cluster, error) {
	var (
		c   Cluster
		err error
	)
	if storage == nil {
		return nil, errStorageMandatory
	}
	c.stor = storage
	c.scheduler = scheduler
	if caPath != "" {
		tlsConfig, errTLS := readTLSConfig(caPath)
		if errTLS != nil {
			return nil, errTLS
		}
		c.tlsConfig = tlsConfig
	}
	c.Healer = DefaultHealer{}
	if scheduler == nil {
		c.scheduler = &roundRobin{lastUsed: -1}
	}
	if len(nodes) > 0 {
		for _, n := range nodes {
			err = c.Register(n)
			if err != nil {
				return &c, err
			}
		}
	}
	return &c, err
}

func readTLSConfig(caPath string) (*tls.Config, error) {
	certPEMBlock, errCert := ioutil.ReadFile(filepath.Join(caPath, "cert.pem"))
	if errCert != nil {
		return nil, errCert
	}
	keyPEMBlock, errCert := ioutil.ReadFile(filepath.Join(caPath, "key.pem"))
	if errCert != nil {
		return nil, errCert
	}
	caPEMCert, errCert := ioutil.ReadFile(filepath.Join(caPath, "ca.pem"))
	if errCert != nil {
		return nil, errCert
	}
	tlsCert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEMCert) {
		return nil, errors.New("Could not add RootCA pem")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      caPool,
	}, nil
}

// Register adds new nodes to the cluster.
func (c *Cluster) Register(node Node) error {
	if node.Address == "" {
		return errors.New("Invalid address")
	}
	node.defTLSConfig = c.tlsConfig
	err := c.runHooks(HookEventBeforeNodeRegister, &node)
	if err != nil {
		return err
	}
	return c.storage().StoreNode(node)
}

func (c *Cluster) UpdateNode(node Node) (Node, error) {
	_, err := c.storage().RetrieveNode(node.Address)
	if err != nil {
		return Node{}, err
	}
	unlock, err := c.lockWithTimeout(node.Address, false)
	if err != nil {
		return Node{}, err
	}
	defer unlock()
	dbNode, err := c.storage().RetrieveNode(node.Address)
	if err != nil {
		return Node{}, err
	}
	if node.CreationStatus != "" && node.CreationStatus != dbNode.CreationStatus {
		dbNode.CreationStatus = node.CreationStatus
	}
	for k, v := range node.Metadata {
		if v == "" {
			delete(dbNode.Metadata, k)
		} else {
			dbNode.Metadata[k] = v
		}
	}
	dbNode.defTLSConfig = c.tlsConfig
	return dbNode, c.storage().UpdateNode(dbNode)
}

// Unregister removes nodes from the cluster.
func (c *Cluster) Unregister(address string) error {
	err := c.runHookForAddr(HookEventBeforeNodeUnregister, address)
	if err != nil {
		return err
	}
	return c.storage().RemoveNode(address)
}

func (c *Cluster) UnregisterNodes(addresses ...string) error {
	for _, address := range addresses {
		err := c.runHookForAddr(HookEventBeforeNodeUnregister, address)
		if err != nil {
			return err
		}
	}
	return c.storage().RemoveNodes(addresses)
}

func (c *Cluster) UnfilteredNodes() ([]Node, error) {
	nodes, err := c.storage().RetrieveNodes()
	if err != nil {
		return nil, err
	}
	return c.setTLSConfigInNodes(nodes), nil
}

func (c *Cluster) Nodes() ([]Node, error) {
	nodes, err := c.storage().RetrieveNodes()
	if err != nil {
		return nil, err
	}
	return NodeList(c.setTLSConfigInNodes(nodes)).filterDisabled(), nil
}

func (c *Cluster) NodesForMetadata(metadata map[string]string) ([]Node, error) {
	nodes, err := c.storage().RetrieveNodesByMetadata(metadata)
	if err != nil {
		return nil, err
	}
	return NodeList(c.setTLSConfigInNodes(nodes)).filterDisabled(), nil
}

func (c *Cluster) GetNode(address string) (Node, error) {
	n, err := c.storage().RetrieveNode(address)
	if err != nil {
		return Node{}, err
	}
	n.defTLSConfig = c.tlsConfig
	return n, nil
}

func (c *Cluster) setTLSConfigInNodes(nodes []Node) []Node {
	for i := range nodes {
		nodes[i].defTLSConfig = c.tlsConfig
	}
	return nodes
}

func (c *Cluster) UnfilteredNodesForMetadata(metadata map[string]string) ([]Node, error) {
	nodes, err := c.storage().RetrieveNodesByMetadata(metadata)
	if err != nil {
		return nil, err
	}
	return c.setTLSConfigInNodes(nodes), nil
}

func (c *Cluster) StartActiveMonitoring(updateInterval time.Duration) {
	c.monitoringDone = make(chan bool)
	go c.runActiveMonitoring(updateInterval)
}

func (c *Cluster) StopActiveMonitoring() {
	if c.monitoringDone != nil {
		c.monitoringDone <- true
	}
}

func (c *Cluster) runPingForHost(addr string, wg *sync.WaitGroup) {
	defer wg.Done()
	client, err := c.getNodeByAddr(addr)
	if err != nil {
		log.Errorf("[active-monitoring]: error creating client: %s", err.Error())
		return
	}
	client.HTTPClient = clientWithTimeout(shortDialTimeout, shortTimeout, client.TLSConfig)
	err = client.Ping()
	if err == nil {
		c.handleNodeSuccess(addr)
	} else {
		log.Errorf("[active-monitoring]: error in ping: %s", err.Error())
		c.handleNodeError(addr, err, true)
	}
}

func (c *Cluster) runActiveMonitoring(updateInterval time.Duration) {
	log.Debugf("[active-monitoring]: active monitoring enabled, pinging hosts every %d seconds", updateInterval/time.Second)
	for {
		var nodes []Node
		var err error
		nodes, err = c.UnfilteredNodes()
		if err != nil {
			log.Errorf("[active-monitoring]: error in UnfilteredNodes: %s", err.Error())
		}
		wg := sync.WaitGroup{}
		for _, node := range nodes {
			wg.Add(1)
			go c.runPingForHost(node.Address, &wg)
		}
		wg.Wait()
		select {
		case <-c.monitoringDone:
			return
		case <-time.After(updateInterval):
		}
	}
}

func (c *Cluster) lockWithTimeout(addr string, isFailure bool) (func(), error) {
	lockTimeout := 3 * time.Minute
	locked, err := c.storage().LockNodeForHealing(addr, isFailure, lockTimeout)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, errHealerInProgress
	}
	doneKeepAlive := make(chan bool)
	go func() {
		for {
			select {
			case <-doneKeepAlive:
				return
			case <-time.After(30 * time.Second):
			}
			c.storage().ExtendNodeLock(addr, lockTimeout)
		}
	}()
	return func() {
		doneKeepAlive <- true
		c.storage().UnlockNode(addr)
	}, nil
}

func (c *Cluster) handleNodeError(addr string, lastErr error, incrementFailures bool) error {
	unlock, err := c.lockWithTimeout(addr, true)
	if err != nil {
		return err
	}
	go func() {
		defer unlock()
		node, err := c.storage().RetrieveNode(addr)
		if err != nil {
			return
		}
		node.updateError(lastErr, incrementFailures)
		duration := c.Healer.HandleError(&node)
		if duration > 0 {
			node.updateDisabled(time.Now().Add(duration))
		}
		c.storage().UpdateNode(node)
		if fn := nodeUpdatedOnError.Val(); fn != nil {
			fn()
		}
	}()
	return nil
}

// Modified by tests
var nodeUpdatedOnError nodeUpdatedHook

type nodeUpdatedHook struct {
	atomic.Value
}

func (v *nodeUpdatedHook) Val() func() {
	if fn := v.Load(); fn != nil {
		return fn.(func())
	}
	return nil
}

func (c *Cluster) handleNodeSuccess(addr string) error {
	unlock, err := c.lockWithTimeout(addr, false)
	if err != nil {
		return err
	}
	defer unlock()
	node, err := c.storage().RetrieveNode(addr)
	if err != nil {
		return err
	}
	node.updateSuccess()
	return c.storage().UpdateNode(node)
}

func (c *Cluster) storage() Storage {
	return c.stor
}

type nodeFunc func(node) (interface{}, error)

func (c *Cluster) runOnNodes(fn nodeFunc, errNotFound error, wait bool, nodeAddresses ...string) (interface{}, error) {
	if len(nodeAddresses) == 0 {
		nodes, err := c.Nodes()
		if err != nil {
			return nil, err
		}
		nodeAddresses = make([]string, len(nodes))
		for i, node := range nodes {
			nodeAddresses[i] = node.Address
		}
	}
	var wg sync.WaitGroup
	finish := make(chan int8, len(nodeAddresses))
	errChan := make(chan error, len(nodeAddresses))
	result := make(chan interface{}, len(nodeAddresses))
	for _, addr := range nodeAddresses {
		wg.Add(1)
		client, err := c.getNodeByAddr(addr)
		if err != nil {
			return nil, err
		}
		go func(n node) {
			defer wg.Done()
			value, err := fn(n)
			if err == nil {
				result <- value
			} else if e, ok := err.(*docker.Error); ok && e.Status == http.StatusNotFound {
				return
			} else if !reflect.DeepEqual(err, errNotFound) {
				errChan <- wrapError(n, err)
			}
		}(client)
	}
	if wait {
		wg.Wait()
		select {
		case value := <-result:
			return value, nil
		case err := <-errChan:
			return nil, err
		default:
			return nil, errNotFound
		}
	}
	go func() {
		wg.Wait()
		close(finish)
	}()
	select {
	case value := <-result:
		return value, nil
	case err := <-errChan:
		return nil, err
	case <-finish:
		select {
		case value := <-result:
			return value, nil
		default:
			return nil, errNotFound
		}
	}
}

func clientWithTimeout(dialTimeout time.Duration, fullTimeout time.Duration, tlsConfig *tls.Config) *http.Client {
	transport := http.Transport{
		Dial: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: dialTimeout,
		MaxIdleConnsPerHost: -1,
		DisableKeepAlives:   true,
		TLSClientConfig:     tlsConfig,
	}
	return &http.Client{
		Transport: &transport,
		Timeout:   fullTimeout,
	}
}

func (c *Cluster) StopDryMode() {
	if c.dryServer != nil {
		c.dryServer.Stop()
	}
}

func (c *Cluster) DryMode() error {
	var err error
	c.dryServer, err = testing.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		return err
	}
	oldStor := c.stor
	c.stor = &MapStorage{}
	nodes, err := oldStor.RetrieveNodes()
	if err != nil {
		return err
	}
	for _, node := range nodes {
		err = c.storage().StoreNode(node)
		if err != nil {
			return err
		}
	}
	images, err := oldStor.RetrieveImages()
	if err != nil {
		return err
	}
	for _, img := range images {
		for _, historyEntry := range img.History {
			if historyEntry.ImageId != img.LastId && historyEntry.Node != img.LastNode {
				err = c.PullImage(docker.PullImageOptions{
					Repository: img.Repository,
				}, docker.AuthConfiguration{}, historyEntry.Node)
				if err != nil {
					return err
				}
			}
		}
		err = c.PullImage(docker.PullImageOptions{
			Repository: img.Repository,
		}, docker.AuthConfiguration{}, img.LastNode)
		if err != nil {
			return err
		}
	}
	containers, err := oldStor.RetrieveContainers()
	if err != nil {
		return err
	}
	for _, container := range containers {
		err = c.storage().StoreContainer(container.Id, container.Host)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) getNodeByAddr(address string) (node, error) {
	if c.dryServer != nil {
		address = c.dryServer.URL()
	}
	n, err := c.GetNode(address)
	if err != nil {
		n = Node{Address: address, defTLSConfig: c.tlsConfig}
	}
	client, err := n.Client()
	if err != nil {
		return node{}, err
	}
	return node{addr: address, Client: client}, nil
}

func (c *Cluster) AddHook(evt HookEvent, h Hook) {
	if c.hooks == nil {
		c.hooks = map[HookEvent][]Hook{}
	}
	c.hooks[evt] = append(c.hooks[evt], h)
}

func (c *Cluster) Hooks(evt HookEvent) []Hook {
	if c.hooks == nil {
		return nil
	}
	return c.hooks[evt]
}

func (c *Cluster) runHookForAddr(evt HookEvent, address string) error {
	if c.hooks == nil || len(c.hooks[evt]) == 0 {
		return nil
	}
	node, err := c.storage().RetrieveNode(address)
	if err != nil {
		return err
	}
	node.defTLSConfig = c.tlsConfig
	return c.runHooks(evt, &node)
}

func (c *Cluster) runHooks(evt HookEvent, n *Node) error {
	if c.hooks == nil {
		return nil
	}
	for _, h := range c.hooks[evt] {
		err := h.RunClusterHook(evt, n)
		if err != nil {
			return err
		}
	}
	return nil
}
