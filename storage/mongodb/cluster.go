// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/provision"
)

type ClusterStorage struct{}

var _ provision.ClusterStorage = &ClusterStorage{}

type cluster struct {
	Name        string `bson:"_id"`
	Addresses   []string
	Provisioner string
	CaCert      []byte            `bson:",omitempty"`
	ClientCert  []byte            `bson:",omitempty"`
	ClientKey   []byte            `bson:",omitempty"`
	Pools       []string          `bson:",omitempty"`
	CustomData  map[string]string `bson:",omitempty"`
	CreateData  map[string]string `bson:",omitempty"`
	Default     bool
}

func clustersCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("clusters")
}

func (s *ClusterStorage) Upsert(c provision.Cluster) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := clustersCollection(conn)
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

func (s *ClusterStorage) FindAll() ([]provision.Cluster, error) {
	clusters, err := s.findByQuery(nil)
	if err != nil {
		return nil, err
	}
	if len(clusters) == 0 {
		return nil, provision.ErrNoCluster
	}
	return clusters, nil
}

func (s *ClusterStorage) FindByName(name string) (*provision.Cluster, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var c provision.Cluster
	err = clustersCollection(conn).FindId(name).One(&c)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = provision.ErrClusterNotFound
		}
		return nil, err
	}
	cluster := provision.Cluster(c)
	return &cluster, nil
}

func (s *ClusterStorage) FindByProvisioner(provisioner string) ([]provision.Cluster, error) {
	return s.findByQuery(bson.M{"provisioner": provisioner})
}

func (s *ClusterStorage) FindByPool(provisioner, pool string) (*provision.Cluster, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := clustersCollection(conn)
	var c provision.Cluster
	if pool != "" {
		err = coll.Find(bson.M{"provisioner": provisioner, "pools": pool}).One(&c)
	}
	if pool == "" || err == mgo.ErrNotFound {
		err = coll.Find(bson.M{"provisioner": provisioner, "default": true}).One(&c)
	}
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, provision.ErrNoCluster
		}
		return nil, errors.WithStack(err)
	}
	return &c, nil
}

func (s *ClusterStorage) findByQuery(query bson.M) ([]provision.Cluster, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var clusters []provision.Cluster
	err = clustersCollection(conn).Find(query).All(&clusters)
	if err != nil {
		return nil, err
	}
	provClusters := make([]provision.Cluster, len(clusters))
	for i, c := range clusters {
		provClusters[i] = provision.Cluster(c)
	}
	return provClusters, nil
}

func (s *ClusterStorage) Delete(c provision.Cluster) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = clustersCollection(conn).RemoveId(c.Name)
	if err == mgo.ErrNotFound {
		return provision.ErrClusterNotFound
	}
	return err
}
