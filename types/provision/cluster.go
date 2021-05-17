// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"context"
	"errors"
	"io"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

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
	Local       bool              `json:"local"`
	Default     bool              `json:"default"`
	KubeConfig  *KubeConfig       `json:"kubeConfig"`

	Writer io.Writer `json:"-"`
}

type KubeConfig struct {
	Cluster  clientcmdapi.Cluster  `json:"cluster"`
	AuthInfo clientcmdapi.AuthInfo `json:"user"`
}

type ClusterHelpInfo struct {
	ProvisionerHelp string            `json:"provisioner_help"`
	CustomDataHelp  map[string]string `json:"custom_data_help"`
}

type ClusterService interface {
	Create(context.Context, Cluster) error
	Update(context.Context, Cluster) error
	List(context.Context) ([]Cluster, error)
	FindByName(context.Context, string) (*Cluster, error)
	FindByProvisioner(context.Context, string) ([]Cluster, error)
	FindByPool(ctx context.Context, provisioner, pool string) (*Cluster, error)
	FindByPools(ctx context.Context, provisioner string, pools []string) (map[string]Cluster, error)
	Delete(context.Context, Cluster) error
}

type ClusterStorage interface {
	Upsert(context.Context, Cluster) error
	FindAll(context.Context) ([]Cluster, error)
	FindByName(context.Context, string) (*Cluster, error)
	FindByProvisioner(context.Context, string) ([]Cluster, error)
	FindByPool(context.Context, string, string) (*Cluster, error)
	Delete(context.Context, Cluster) error
}

func (c *Cluster) CleanUpSensitive() {
	c.ClientKey = nil
	delete(c.CustomData, "token")
	delete(c.CustomData, "password")
}

var (
	ErrClusterNotFound = errors.New("cluster not found")
	ErrNoCluster       = errors.New("no cluster")
)
