// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"sync"
	"time"

	"github.com/tsuru/docker-cluster/storage"
)

type MapStorage struct {
	cMap    map[string]string
	eMap    map[string]string
	iMap    map[string]*Image
	nodes   []Node
	nodeMap map[string]*Node
	cMut    sync.Mutex
	iMut    sync.Mutex
	nMut    sync.Mutex
	eMut    sync.Mutex
}

var _ Storage = &MapStorage{}

func (s *MapStorage) StoreContainer(containerID, hostID string) error {
	s.cMut.Lock()
	defer s.cMut.Unlock()
	if s.cMap == nil {
		s.cMap = make(map[string]string)
	}
	s.cMap[containerID] = hostID
	return nil
}

func (s *MapStorage) RetrieveContainer(containerID string) (string, error) {
	s.cMut.Lock()
	defer s.cMut.Unlock()
	host, ok := s.cMap[containerID]
	if !ok {
		return "", storage.ErrNoSuchContainer
	}
	return host, nil
}

func (s *MapStorage) RemoveContainer(containerID string) error {
	s.cMut.Lock()
	defer s.cMut.Unlock()
	delete(s.cMap, containerID)
	s.eMut.Lock()
	defer s.eMut.Unlock()
	for k, v := range s.eMap {
		if v == containerID {
			delete(s.eMap, k)
		}
	}
	return nil
}

func (s *MapStorage) RetrieveContainers() ([]Container, error) {
	s.cMut.Lock()
	defer s.cMut.Unlock()
	entries := make([]Container, 0, len(s.cMap))
	for k, v := range s.cMap {
		entries = append(entries, Container{Id: k, Host: v})
	}
	return entries, nil
}

func (s *MapStorage) StoreImage(repo, id, host string) error {
	s.iMut.Lock()
	defer s.iMut.Unlock()
	if s.iMap == nil {
		s.iMap = make(map[string]*Image)
	}
	img, _ := s.iMap[repo]
	if img == nil {
		img = &Image{Repository: repo, History: []ImageHistory{}}
		s.iMap[repo] = img
	}
	hasId := false
	for _, entry := range img.History {
		if entry.ImageId == id && entry.Node == host {
			hasId = true
			break
		}
	}
	if !hasId {
		img.History = append(img.History, ImageHistory{Node: host, ImageId: id})
	}
	img.LastNode = host
	img.LastId = id
	return nil
}

func (s *MapStorage) SetImageDigest(repo, digest string) error {
	s.iMut.Lock()
	defer s.iMut.Unlock()
	img, _ := s.iMap[repo]
	if img == nil {
		img = &Image{Repository: repo, History: []ImageHistory{}}
		s.iMap[repo] = img
	}
	img.LastDigest = digest
	return nil

}

func (s *MapStorage) RetrieveImage(repo string) (Image, error) {
	s.iMut.Lock()
	defer s.iMut.Unlock()
	image, ok := s.iMap[repo]
	if !ok {
		return Image{}, storage.ErrNoSuchImage
	}
	if len(image.History) == 0 {
		return Image{}, storage.ErrNoSuchImage
	}
	return *image, nil
}

func (s *MapStorage) RemoveImage(repo, id, host string) error {
	s.iMut.Lock()
	defer s.iMut.Unlock()
	image, ok := s.iMap[repo]
	if !ok {
		return storage.ErrNoSuchImage
	}
	newHistory := []ImageHistory{}
	for _, entry := range image.History {
		if entry.ImageId != id || entry.Node != host {
			newHistory = append(newHistory, entry)
		}
	}
	image.History = newHistory
	return nil
}

func (s *MapStorage) RetrieveImages() ([]Image, error) {
	s.iMut.Lock()
	defer s.iMut.Unlock()
	images := make([]Image, 0, len(s.iMap))
	for _, img := range s.iMap {
		images = append(images, *img)
	}
	return images, nil
}

func (s *MapStorage) updateNodeMap() {
	s.nodeMap = make(map[string]*Node)
	for i := range s.nodes {
		s.nodeMap[s.nodes[i].Address] = &s.nodes[i]
	}
}

func (s *MapStorage) StoreNode(node Node) error {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	for _, n := range s.nodes {
		if n.Address == node.Address {
			return storage.ErrDuplicatedNodeAddress
		}
	}
	if node.Metadata == nil {
		node.Metadata = make(map[string]string)
	}
	s.nodes = append(s.nodes, node)
	s.updateNodeMap()
	return nil
}

func deepCopyNode(n Node) Node {
	newMap := map[string]string{}
	for k, v := range n.Metadata {
		newMap[k] = v
	}
	n.Metadata = newMap
	return n
}

func (s *MapStorage) RetrieveNodes() ([]Node, error) {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	dst := make([]Node, len(s.nodes))
	for i := range s.nodes {
		dst[i] = deepCopyNode(s.nodes[i])
	}
	return dst, nil
}

func (s *MapStorage) RetrieveNode(address string) (Node, error) {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	if s.nodeMap == nil {
		s.nodeMap = make(map[string]*Node)
	}
	node, ok := s.nodeMap[address]
	if !ok {
		return Node{}, storage.ErrNoSuchNode
	}
	return deepCopyNode(*node), nil
}

func (s *MapStorage) UpdateNode(node Node) error {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	if s.nodeMap == nil {
		s.nodeMap = make(map[string]*Node)
	}
	_, ok := s.nodeMap[node.Address]
	if !ok {
		return storage.ErrNoSuchNode
	}
	*s.nodeMap[node.Address] = node
	return nil
}

func hasAllMetadata(base, wanted map[string]string) bool {
	for key, value := range wanted {
		nodeVal := base[key]
		if nodeVal != value {
			return false
		}
	}
	return true
}

func (s *MapStorage) RetrieveNodesByMetadata(metadata map[string]string) ([]Node, error) {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	filteredNodes := []Node{}
	for _, node := range s.nodes {
		if hasAllMetadata(node.Metadata, metadata) {
			filteredNodes = append(filteredNodes, node)
		}
	}
	return filteredNodes, nil
}

func (s *MapStorage) RemoveNode(addr string) error {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	index := -1
	for i, node := range s.nodes {
		if node.Address == addr {
			index = i
		}
	}
	if index < 0 {
		return storage.ErrNoSuchNode
	}
	copy(s.nodes[index:], s.nodes[index+1:])
	s.nodes = s.nodes[:len(s.nodes)-1]
	s.updateNodeMap()
	return nil
}

func (s *MapStorage) RemoveNodes(addresses []string) error {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	addrMap := map[string]struct{}{}
	for _, addr := range addresses {
		addrMap[addr] = struct{}{}
	}
	dup := make([]Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		if _, ok := addrMap[node.Address]; !ok {
			dup = append(dup, node)
		}
	}
	if len(dup) == len(s.nodes) {
		return storage.ErrNoSuchNode
	}
	s.nodes = dup
	s.updateNodeMap()
	return nil
}

func (s *MapStorage) LockNodeForHealing(address string, isFailure bool, timeout time.Duration) (bool, error) {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	n, present := s.nodeMap[address]
	if !present {
		return false, storage.ErrNoSuchNode
	}
	now := time.Now().UTC()
	if n.Healing.LockedUntil.After(now) {
		return false, nil
	}
	n.Healing.LockedUntil = now.Add(timeout)
	n.Healing.IsFailure = isFailure
	return true, nil
}

func (s *MapStorage) ExtendNodeLock(address string, timeout time.Duration) error {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	n, present := s.nodeMap[address]
	if !present {
		return storage.ErrNoSuchNode
	}
	now := time.Now().UTC()
	n.Healing.LockedUntil = now.Add(timeout)
	return nil
}

func (s *MapStorage) UnlockNode(address string) error {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	n, present := s.nodeMap[address]
	if !present {
		return storage.ErrNoSuchNode
	}
	n.Healing = HealingData{}
	return nil
}

func (s *MapStorage) StoreExec(execID, containerID string) error {
	s.eMut.Lock()
	defer s.eMut.Unlock()
	if s.eMap == nil {
		s.eMap = make(map[string]string)
	}
	s.eMap[execID] = containerID
	return nil
}

func (s *MapStorage) RetrieveExec(execID string) (containerID string, err error) {
	s.eMut.Lock()
	defer s.eMut.Unlock()
	containerID, ok := s.eMap[execID]
	if !ok {
		return "", storage.ErrNoSuchExec
	}
	return containerID, nil
}
