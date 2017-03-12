// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

const (
	kubernetesCollectionName = "kubernetescluster"
	defaultTimeout           = time.Minute
)

var errNoCluster = errors.New("no kubernetes cluster")

var clientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(conf)
}

type clusterData struct {
	Address    string `bson:"_id"`
	CaCert     []byte
	ClientCert []byte
	ClientKey  []byte
}

func addClusterNode(opts provision.AddNodeOptions) error {
	coll, err := clusterAddrCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.Insert(clusterData{
		Address:    opts.Address,
		CaCert:     opts.CaCert,
		ClientCert: opts.ClientCert,
		ClientKey:  opts.ClientKey,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func removeClusterNode(addr string) error {
	coll, err := clusterAddrCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.RemoveId(addr)
}

func getClusterRestConfig() (*rest.Config, error) {
	coll, err := clusterAddrCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var data clusterData
	err = coll.Find(nil).One(&data)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, errNoCluster
		}
		return nil, errors.WithStack(err)
	}
	gv, err := unversioned.ParseGroupVersion("/v1")
	if err != nil {
		return nil, err
	}
	return &rest.Config{
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &gv,
			NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: api.Codecs},
		},
		Host: data.Address,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   data.CaCert,
			CertData: data.ClientCert,
			KeyData:  data.ClientKey,
		},
		Timeout: defaultTimeout,
	}, nil
}

func getClusterClientWithCfg() (kubernetes.Interface, *rest.Config, error) {
	cfg, err := getClusterRestConfig()
	if err != nil {
		return nil, nil, err
	}
	client, err := clientForConfig(cfg)
	return client, cfg, errors.WithStack(err)
}

func getClusterClient() (kubernetes.Interface, error) {
	client, _, err := getClusterClientWithCfg()
	return client, err
}

func clusterAddrCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn.Collection(kubernetesCollectionName), nil
}
