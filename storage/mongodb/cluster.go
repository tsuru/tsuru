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

type clusterStorage struct{}

var _ provision.ClusterStorage = &clusterStorage{}

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
	return conn.Collection("provisioner_clusters")
}

func (s *clusterStorage) Upsert(c provision.Cluster) error {
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

func (s *clusterStorage) FindAll() ([]provision.Cluster, error) {
	return s.findByQuery(nil)
}

func (s *clusterStorage) FindByName(name string) (*provision.Cluster, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var c cluster
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

func (s *clusterStorage) FindByProvisioner(provisioner string) ([]provision.Cluster, error) {
	return s.findByQuery(bson.M{"provisioner": provisioner})
}

func (s *clusterStorage) FindByPool(provisioner, pool string) (*provision.Cluster, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := clustersCollection(conn)
	var c cluster
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
	cluster := provision.Cluster(c)
	return &cluster, nil
}

func (s *clusterStorage) findByQuery(query bson.M) ([]provision.Cluster, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var clusters []cluster
	err = clustersCollection(conn).Find(query).All(&clusters)
	if err != nil {
		return nil, err
	}
	if len(clusters) == 0 {
		return nil, provision.ErrNoCluster
	}
	provClusters := make([]provision.Cluster, len(clusters))
	for i, c := range clusters {
		provClusters[i] = provision.Cluster(c)
	}
	return provClusters, nil
}

func (s *clusterStorage) Delete(c provision.Cluster) error {
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
