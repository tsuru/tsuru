// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"strings"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/storage"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/validation"
)

var (
	ErrClusterNotFound = errors.New("cluster not found")
	ErrNoCluster       = errors.New("no cluster")
)

type Cluster struct {
	Name        string            `json:"name" bson:"_id"`
	Addresses   []string          `json:"addresses"`
	Provisioner string            `json:"provisioner"`
	CaCert      []byte            `json:"cacert" bson:",omitempty"`
	ClientCert  []byte            `json:"clientcert" bson:",omitempty"`
	ClientKey   []byte            `json:"-" bson:",omitempty"`
	Pools       []string          `json:"pools" bson:",omitempty"`
	CustomData  map[string]string `json:"custom_data" bson:",omitempty"`
	CreateData  map[string]string `json:"create_data" bson:",omitempty"`
	Default     bool              `json:"default"`
}

type InitClusterProvisioner interface {
	InitializeCluster(c *Cluster) error
}

type clusterService struct {
	storage provTypes.ClusterStorage
}

func ClusterService() (provTypes.ClusterService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &clusterService{
		storage: dbDriver.ClusterStorage,
	}, nil
}

func (s *clusterService) Save(c provTypes.Cluster) error {
	err := s.validate(c)
	if err != nil {
		return err
	}
	return s.storage.Upsert(c)
}

func (s *clusterService) List() ([]provTypes.Cluster, error) {
	return s.storage.FindAll()
}

func (s *clusterService) FindByName(name string) (*provTypes.Cluster, error) {
	return s.storage.FindByName(name)
}

func (s *clusterService) FindByProvisioner(prov string) ([]provTypes.Cluster, error) {
	return s.storage.FindByProvisioner(prov)
}

func (s *clusterService) FindByPool(prov, pool string) (*provTypes.Cluster, error) {
	return s.storage.FindByPool(prov, pool)
}

func (s *clusterService) Delete(c provTypes.Cluster) error {
	return s.storage.Delete(c)
}

func (s *clusterService) validate(c provTypes.Cluster) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "cluster name is mandatory"})
	}
	if !validation.ValidateName(c.Name) {
		msg := "Invalid cluster name, cluster name should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return errors.WithStack(&tsuruErrors.ValidationError{Message: msg})
	}
	if c.Provisioner == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "provisioner name is mandatory"})
	}
	prov, err := provision.Get(c.Provisioner)
	if err != nil {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: err.Error()})
	}
	if len(c.Pools) > 0 {
		if c.Default {
			return errors.WithStack(&tsuruErrors.ValidationError{Message: "cannot have both pools and default set"})
		}
	} else {
		if !c.Default {
			return errors.WithStack(&tsuruErrors.ValidationError{Message: "either default or a list of pools must be set"})
		}
	}
	if clusterProv, ok := prov.(InitClusterProvisioner); ok {
		cc := Cluster(c)
		err = clusterProv.InitializeCluster(&cc)
		if err != nil {
			return err
		}
	}
	return nil
}

func clusterCollection() (*dbStorage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn.ProvisionerClusters(), nil
}

func (c *Cluster) validate() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "cluster name is mandatory"})
	}
	if !validation.ValidateName(c.Name) {
		msg := "Invalid cluster name, cluster name should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return errors.WithStack(&tsuruErrors.ValidationError{Message: msg})
	}
	if c.Provisioner == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "provisioner name is mandatory"})
	}
	prov, err := provision.Get(c.Provisioner)
	if err != nil {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: err.Error()})
	}
	if len(c.Pools) > 0 {
		if c.Default {
			return errors.WithStack(&tsuruErrors.ValidationError{Message: "cannot have both pools and default set"})
		}
	} else {
		if !c.Default {
			return errors.WithStack(&tsuruErrors.ValidationError{Message: "either default or a list of pools must be set"})
		}
	}
	if clusterProv, ok := prov.(InitClusterProvisioner); ok {
		err = clusterProv.InitializeCluster(c)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) Save() error {
	err := c.validate()
	if err != nil {
		return err
	}
	coll, err := clusterCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	updates := bson.M{}
	if len(c.Pools) > 0 {
		updates["$pullAll"] = bson.M{"pools": c.Pools}
	}
	if c.Default {
		updates["$set"] = bson.M{"default": false}
	}
	if len(updates) > 0 {
		_, err = coll.UpdateAll(bson.M{"provisioner": c.Provisioner}, updates)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	_, err = coll.UpsertId(c.Name, c)
	return errors.WithStack(err)
}

func AllClusters() ([]*Cluster, error) {
	return listClusters(nil)
}

func ByName(clusterName string) (*Cluster, error) {
	coll, err := clusterCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var c *Cluster
	err = coll.FindId(clusterName).One(&c)
	if err == mgo.ErrNotFound {
		return nil, ErrClusterNotFound
	}
	return c, err
}

func DeleteCluster(clusterName string) error {
	coll, err := clusterCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.RemoveId(clusterName)
	if err != nil {
		if err == mgo.ErrNotFound {
			return ErrClusterNotFound
		}
	}
	return err
}

func ForProvisioner(provisioner string) ([]*Cluster, error) {
	return listClusters(bson.M{"provisioner": provisioner})
}

func ForPool(provisioner, pool string) (*Cluster, error) {
	coll, err := clusterCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var c Cluster
	if pool != "" {
		err = coll.Find(bson.M{"provisioner": provisioner, "pools": pool}).One(&c)
	}
	if pool == "" || err == mgo.ErrNotFound {
		err = coll.Find(bson.M{"provisioner": provisioner, "default": true}).One(&c)
	}
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrNoCluster
		}
		return nil, errors.WithStack(err)
	}
	return &c, nil
}

func listClusters(query bson.M) ([]*Cluster, error) {
	coll, err := clusterCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var clusters []*Cluster
	err = coll.Find(query).All(&clusters)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(clusters) == 0 {
		return nil, ErrNoCluster
	}
	return clusters, nil
}
