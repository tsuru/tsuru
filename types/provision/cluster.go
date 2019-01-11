// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import "errors"

// Cluster represents a cluster of nodes.
type Cluster struct {
	Name        string            `json:"name"`
	Addresses   []string          `json:"addresses"`
	Provisioner string            `json:"provisioner"`
	CaCert      []byte            `json:"cacert"`
	ClientCert  []byte            `json:"clientcert"`
	ClientKey   []byte            `json:"clientkey"`
	Pools       []string          `json:"pools"`
	CustomData  map[string]string `json:"custom_data"`
	CreateData  map[string]string `json:"create_data"`
	Default     bool              `json:"default"`
}

type ClusterService interface {
	Create(Cluster) error
	Update(Cluster) error
	List() ([]Cluster, error)
	FindByName(string) (*Cluster, error)
	FindByProvisioner(string) ([]Cluster, error)
	FindByPool(string, string) (*Cluster, error)
	FindByPools(string, []string) (map[string]Cluster, error)
	Delete(Cluster) error
}

type ClusterStorage interface {
	Upsert(Cluster) error
	FindAll() ([]Cluster, error)
	FindByName(string) (*Cluster, error)
	FindByProvisioner(string) ([]Cluster, error)
	FindByPool(string, string) (*Cluster, error)
	Delete(Cluster) error
}

var (
	ErrClusterNotFound = errors.New("cluster not found")
	ErrNoCluster       = errors.New("no cluster")
)
