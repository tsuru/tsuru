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

func getClusterClient() (kubernetes.Interface, error) {
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
	client, err := clientForConfig(&rest.Config{
		Host: data.Address,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   data.CaCert,
			CertData: data.ClientCert,
			KeyData:  data.ClientKey,
		},
		Timeout: defaultTimeout,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return client, nil
}

func clusterAddrCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn.Collection(kubernetesCollectionName), nil
}
