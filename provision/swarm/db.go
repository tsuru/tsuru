// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"crypto/tls"
	"crypto/x509"
	"math/rand"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type notFoundError struct{ error }

func (e notFoundError) NotFound() bool {
	return true
}

var errNoSwarmNode = notFoundError{errors.New("no swarm nodes available")}

const (
	uniqueDocumentID       = "swarm"
	swarmCollectionName    = "swarmnodes"
	swarmSecCollectionName = "swarmsec"
	nodeRetryCount         = 3
)

type NodeAddrs struct {
	UniqueID  string `bson:"_id"`
	Addresses []string
}

func chooseDBSwarmNode() (*docker.Client, error) {
	coll, err := nodeAddrCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var addrs NodeAddrs
	err = coll.FindId(uniqueDocumentID).One(&addrs)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.WithStack(err)
	}
	if len(addrs.Addresses) == 0 {
		return nil, errors.Wrap(errNoSwarmNode, "")
	}
	var client *docker.Client
	initialIdx := rand.Intn(len(addrs.Addresses))
	var i int
	for ; i < nodeRetryCount; i++ {
		idx := (initialIdx + i) % len(addrs.Addresses)
		addr := addrs.Addresses[idx]
		client, err = newClient(addr)
		if err != nil {
			return nil, err
		}
		err = client.Ping()
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if i > 0 {
		updateDBSwarmNodes(client)
	}
	return client, nil
}

func updateDBSwarmNodes(client *docker.Client) error {
	nodes, err := listValidNodes(client)
	if err != nil {
		return errors.WithStack(err)
	}
	var addrs []string
	for _, n := range nodes {
		if n.ManagerStatus == nil {
			continue
		}
		addr := n.Spec.Annotations.Labels[labelNodeDockerAddr.String()]
		if addr == "" {
			continue
		}
		addrs = append(addrs, addr)
	}
	coll, err := nodeAddrCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(uniqueDocumentID, bson.M{"$set": bson.M{"addresses": addrs}})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func removeDBSwarmNodes() error {
	coll, err := nodeAddrCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.RemoveId(uniqueDocumentID)
	if err != nil && err != mgo.ErrNotFound {
		return errors.WithStack(err)
	}
	return nil
}

type NodeSec struct {
	Address    string `bson:"_id"`
	CaCert     []byte
	ClientCert []byte
	ClientKey  []byte
}

func addNodeCredentials(opts provision.AddNodeOptions) error {
	if opts.CaCert == nil && opts.ClientCert == nil && opts.ClientKey == nil {
		return nil
	}
	secColl, err := nodeSecurityCollection()
	if err != nil {
		return err
	}
	defer secColl.Close()
	data := NodeSec{
		Address:    opts.Address,
		CaCert:     opts.CaCert,
		ClientCert: opts.ClientCert,
		ClientKey:  opts.ClientKey,
	}
	_, err = secColl.UpsertId(data.Address, data)
	return errors.WithStack(err)
}

func getNodeCredentials(address string) (*tls.Config, error) {
	secColl, err := nodeSecurityCollection()
	if err != nil {
		return nil, err
	}
	defer secColl.Close()
	var data NodeSec
	err = secColl.FindId(address).One(&data)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	tlsCert, err := tls.X509KeyPair(data.ClientCert, data.ClientKey)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(data.CaCert) {
		return nil, errors.New("could not add RootCA pem")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      caPool,
	}, nil
}

func nodeAddrCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn.Collection(swarmCollectionName), nil
}

func nodeSecurityCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn.Collection(swarmSecCollectionName), nil
}
