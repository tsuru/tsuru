// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"io"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/provision"
)

const clusterCollection = "provisioner_clusters"

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
	Default     bool
	Writer      io.Writer `bson:"-"`
}

func clustersCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection(clusterCollection)
}

func (s *clusterStorage) Upsert(ctx context.Context, c provision.Cluster) error {
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
		query := bson.M{"provisioner": c.Provisioner}

		span := newMongoDBSpan(ctx, mongoSpanUpdateAll, clusterCollection)
		span.SetQueryStatement(query)
		defer span.Finish()

		_, err = coll.UpdateAll(query, updates)
		if err != nil {
			span.SetError(err)
			return errors.WithStack(err)
		}
	}

	span := newMongoDBSpan(ctx, mongoSpanUpsert, clusterCollection)
	span.SetMongoID(c.Name)
	defer span.Finish()

	_, err = coll.UpsertId(c.Name, cluster(c))
	if err != nil {
		span.SetError(err)
		return errors.WithStack(err)
	}

	return nil
}

func (s *clusterStorage) FindAll(ctx context.Context) ([]provision.Cluster, error) {
	return s.findByQuery(ctx, nil)
}

func (s *clusterStorage) FindByName(ctx context.Context, name string) (*provision.Cluster, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var c cluster

	span := newMongoDBSpan(ctx, mongoSpanFindID, clusterCollection)
	span.SetMongoID(name)
	defer span.Finish()

	err = clustersCollection(conn).FindId(name).One(&c)
	if err != nil {
		if err == mgo.ErrNotFound {
			span.LogKV("event", provision.ErrClusterNotFound.Error())
			return nil, provision.ErrClusterNotFound
		}
		span.SetError(err)
		return nil, err
	}
	cluster := provision.Cluster(c)
	return &cluster, nil
}

func (s *clusterStorage) FindByProvisioner(ctx context.Context, provisioner string) ([]provision.Cluster, error) {
	return s.findByQuery(ctx, bson.M{"provisioner": provisioner})
}

func (s *clusterStorage) FindByPool(ctx context.Context, provisioner, pool string) (*provision.Cluster, error) {

	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := clustersCollection(conn)
	var c cluster
	if pool != "" {
		query := bson.M{"provisioner": provisioner, "pools": pool}

		span := newMongoDBSpan(ctx, mongoSpanFind, clusterCollection)
		span.SetQueryStatement(query)

		err = coll.Find(query).One(&c)
		if err != mgo.ErrNotFound {
			span.SetError(err)
		}
	}

	if pool == "" || err == mgo.ErrNotFound {
		query := bson.M{"provisioner": provisioner, "default": true}

		span := newMongoDBSpan(ctx, mongoSpanFind, clusterCollection)
		span.SetQueryStatement(query)

		err = coll.Find(query).One(&c)
		if err != mgo.ErrNotFound {
			span.SetError(err)
		}
		span.Finish()
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

func (s *clusterStorage) findByQuery(ctx context.Context, query bson.M) ([]provision.Cluster, error) {
	span := newMongoDBSpan(ctx, mongoSpanFind, clusterCollection)
	span.SetQueryStatement(query)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	defer conn.Close()
	var clusters []cluster
	err = clustersCollection(conn).Find(query).All(&clusters)
	if err != nil {
		span.SetError(err)
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

func (s *clusterStorage) Delete(ctx context.Context, c provision.Cluster) error {
	span := newMongoDBSpan(ctx, mongoSpanDelete, clusterCollection)
	span.SetMongoID(c.Name)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = clustersCollection(conn).RemoveId(c.Name)
	span.SetError(err)
	if err == mgo.ErrNotFound {
		return provision.ErrClusterNotFound
	}
	return err
}
