// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"math/rand"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

const (
	kubeClusterCollection = "kubernetes_clusters"
	defaultTimeout        = time.Minute
)

var (
	ErrClusterNotFound = errors.New("cluster not found")
	ErrNoCluster       = errors.New("no kubernetes cluster")
)

var clientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(conf)
}

type Cluster struct {
	kubernetes.Interface `json:"-" bson:"-"`
	Name                 string   `json:"name" bson:"_id"`
	Addresses            []string `json:"addresses"`
	CaCert               []byte   `json:"cacert" bson:",omitempty"`
	ClientCert           []byte   `json:"clientcert" bson:",omitempty"`
	ClientKey            []byte   `json:"-" bson:",omitempty"`
	Pools                []string `json:"pools" bson:",omitempty"`
	ExplicitNamespace    string   `json:"namespace" bson:"namespace,omitempty"`
	Default              bool     `json:"default"`

	restConfig *rest.Config
}

func clusterCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn.Collection(kubeClusterCollection), nil
}

func (c *Cluster) validate() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "cluster name is mandatory"})
	}
	if len(c.Addresses) == 0 {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "at least one address must be present"})
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
	return c.initClient()
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
		_, err = coll.UpdateAll(nil, updates)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	_, err = coll.UpsertId(c.Name, c)
	return errors.WithStack(err)
}

func (c *Cluster) getRestConfig() (*rest.Config, error) {
	gv, err := unversioned.ParseGroupVersion("/v1")
	if err != nil {
		return nil, err
	}
	addr := c.Addresses[rand.Intn(len(c.Addresses))]
	return &rest.Config{
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &gv,
			NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: api.Codecs},
		},
		Host: addr,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   c.CaCert,
			CertData: c.ClientCert,
			KeyData:  c.ClientKey,
		},
		Timeout: defaultTimeout,
	}, nil
}

func (c *Cluster) initClient() error {
	if c.Interface != nil && c.restConfig != nil {
		return nil
	}
	cfg, err := c.getRestConfig()
	if err != nil {
		return err
	}
	client, err := clientForConfig(cfg)
	if err != nil {
		return err
	}
	c.Interface = client
	c.restConfig = cfg
	return nil
}

func (c *Cluster) namespace() string {
	if c.ExplicitNamespace == "" {
		return "default"
	}
	return c.ExplicitNamespace
}

func AllClusters() ([]*Cluster, error) {
	coll, err := clusterCollection()
	if err != nil {
		return nil, err
	}
	var clusters []*Cluster
	err = coll.Find(nil).All(&clusters)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(clusters) == 0 {
		return nil, ErrNoCluster
	}
	for i := range clusters {
		err = clusters[i].initClient()
		if err != nil {
			return nil, err
		}
	}
	return clusters, nil
}

func DeleteCluster(clusterName string) error {
	coll, err := clusterCollection()
	if err != nil {
		return err
	}
	err = coll.RemoveId(clusterName)
	if err != nil {
		if err == mgo.ErrNotFound {
			return ErrClusterNotFound
		}
	}
	return err
}

func clusterForPool(pool string) (*Cluster, error) {
	coll, err := clusterCollection()
	if err != nil {
		return nil, err
	}
	var c Cluster
	if pool != "" {
		err = coll.Find(bson.M{"pools": pool}).One(&c)
	}
	if pool == "" || err == mgo.ErrNotFound {
		err = coll.Find(bson.M{"default": true}).One(&c)
	}
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrNoCluster
		}
		return nil, errors.WithStack(err)
	}
	err = c.initClient()
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func forEachCluster(fn func(cluster *Cluster) error) error {
	clusters, err := AllClusters()
	if err != nil {
		return err
	}
	errors := tsuruErrors.NewMultiError()
	for _, c := range clusters {
		err = fn(c)
		if err != nil {
			errors.Add(err)
		}
	}
	if errors.Len() > 0 {
		return errors
	}
	return nil
}
