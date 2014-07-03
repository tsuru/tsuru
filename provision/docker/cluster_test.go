// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"sync"
)

type mapStorage struct {
	cMap  map[string]string
	iMap  map[string][]string
	nodes []cluster.Node
	cMut  sync.Mutex
	iMut  sync.Mutex
	nMut  sync.Mutex
}

func (s *mapStorage) StoreContainer(containerID, hostID string) error {
	s.cMut.Lock()
	defer s.cMut.Unlock()
	if s.cMap == nil {
		s.cMap = make(map[string]string)
	}
	s.cMap[containerID] = hostID
	return nil
}

func (s *mapStorage) RetrieveContainer(containerID string) (string, error) {
	s.cMut.Lock()
	defer s.cMut.Unlock()
	host, ok := s.cMap[containerID]
	if !ok {
		return "", &docker.NoSuchContainer{ID: containerID}
	}
	return host, nil
}

func (s *mapStorage) RemoveContainer(containerID string) error {
	s.cMut.Lock()
	defer s.cMut.Unlock()
	delete(s.cMap, containerID)
	return nil
}

func (s *mapStorage) StoreImage(imageID, hostID string) error {
	s.iMut.Lock()
	defer s.iMut.Unlock()
	if s.iMap == nil {
		s.iMap = make(map[string][]string)
	}
	s.iMap[imageID] = append(s.iMap[imageID], hostID)
	return nil
}

func (s *mapStorage) RetrieveImage(imageID string) ([]string, error) {
	s.iMut.Lock()
	defer s.iMut.Unlock()
	hosts, ok := s.iMap[imageID]
	if !ok {
		return nil, docker.ErrNoSuchImage
	}
	return hosts, nil
}

func (s *mapStorage) RemoveImage(imageID string) error {
	s.iMut.Lock()
	defer s.iMut.Unlock()
	delete(s.iMap, imageID)
	return nil
}

func (s *mapStorage) StoreNode(node cluster.Node) error {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	s.nodes = append(s.nodes, node)
	return nil
}

func (s *mapStorage) RetrieveNode(id string) (string, error) {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	for _, node := range s.nodes {
		if node.ID == id {
			return node.Address, nil
		}
	}
	return "", errors.New("no such node")
}

func (s *mapStorage) RetrieveNodes() ([]cluster.Node, error) {
	return s.nodes, nil
}

func (s *mapStorage) RemoveNode(id string) error {
	s.nMut.Lock()
	defer s.nMut.Unlock()
	index := -1
	for i, node := range s.nodes {
		if node.ID == id {
			index = i
		}
	}
	if index < 0 {
		return errors.New("no such node")
	}
	copy(s.nodes[index:], s.nodes[index+1:])
	s.nodes = s.nodes[:len(s.nodes)-1]
	return nil
}
