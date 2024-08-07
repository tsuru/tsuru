// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/provision"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	Local       bool              `bson:",omitempty"`
	Default     bool
	KubeConfig  *provision.KubeConfig `bson:",omitempty"`
	HTTPProxy   string                `json:"httpProxy,omitempty"`
}

func (s *clusterStorage) Upsert(ctx context.Context, c provision.Cluster) error {
	collection, err := storagev2.ProvisionerClustersCollection()
	if err != nil {
		return err
	}
	updates := mongoBSON.M{}
	if len(c.Pools) > 0 {
		updates["$pullAll"] = mongoBSON.M{"pools": c.Pools}
	}
	if c.Default {
		updates["$set"] = mongoBSON.M{"default": false}
	}
	if len(updates) > 0 {
		query := mongoBSON.M{"provisioner": c.Provisioner}

		span := newMongoDBSpan(ctx, mongoSpanUpdateAll, collection.Name())
		span.SetQueryStatement(query)
		defer span.Finish()

		_, err = collection.UpdateMany(ctx, query, updates)
		if err != nil {
			span.SetError(err)
			return errors.WithStack(err)
		}
	}

	span := newMongoDBSpan(ctx, mongoSpanUpsert, collection.Name())
	span.SetMongoID(c.Name)
	defer span.Finish()

	_, err = collection.ReplaceOne(ctx, mongoBSON.M{"_id": c.Name}, cluster(c), options.Replace().SetUpsert(true))
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
	collection, err := storagev2.ProvisionerClustersCollection()
	if err != nil {
		return nil, err
	}
	var c cluster

	span := newMongoDBSpan(ctx, mongoSpanFindID, collection.Name())
	span.SetMongoID(name)
	defer span.Finish()

	err = collection.FindOne(ctx, mongoBSON.M{"_id": name}).Decode(&c)
	if err != nil {
		if err == mongo.ErrNoDocuments {
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
	return s.findByQuery(ctx, mongoBSON.M{"provisioner": provisioner})
}

func (s *clusterStorage) FindByPool(ctx context.Context, provisioner, pool string) (*provision.Cluster, error) {
	collection, err := storagev2.ProvisionerClustersCollection()
	if err != nil {
		return nil, err
	}
	var c cluster
	if pool != "" {
		query := mongoBSON.M{"provisioner": provisioner, "pools": pool}

		span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
		span.SetQueryStatement(query)

		err = collection.FindOne(ctx, query).Decode(&c)
		if err != mongo.ErrNoDocuments {
			span.SetError(err)
		}
	}

	if pool == "" || err == mongo.ErrNoDocuments {
		query := mongoBSON.M{"provisioner": provisioner, "default": true}

		span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
		span.SetQueryStatement(query)

		err = collection.FindOne(ctx, query).Decode(&c)
		if err != mongo.ErrNoDocuments {
			span.SetError(err)
		}
		span.Finish()
	}
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, provision.ErrNoCluster
		}
		return nil, errors.WithStack(err)
	}
	cluster := provision.Cluster(c)
	return &cluster, nil
}

func (s *clusterStorage) findByQuery(ctx context.Context, query mongoBSON.M) ([]provision.Cluster, error) {

	collection, err := storagev2.ProvisionerClustersCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

	var clusters []cluster

	cursor, err := collection.Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	err = cursor.All(ctx, &clusters)
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
	collection, err := storagev2.ProvisionerClustersCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanDelete, collection.Name())
	span.SetMongoID(c.Name)
	defer span.Finish()

	_, err = collection.DeleteOne(ctx, mongoBSON.M{"_id": c.Name})
	if err == mgo.ErrNotFound {
		return provision.ErrClusterNotFound
	}
	if err != nil {
		span.SetError(err)
		return errors.WithStack(err)
	}

	return nil
}
