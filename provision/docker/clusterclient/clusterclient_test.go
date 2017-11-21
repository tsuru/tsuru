// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package clusterclient

import (
	"context"
	"testing"

	"github.com/fsouza/go-dockerclient"
	dTesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&S{})

type S struct {
	server *dTesting.DockerServer
	client *docker.Client
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "docker_provision_docker_clusterclient_tests")
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.server, err = dTesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	s.client, err = docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Stop()
}

func (s *S) collection() *storage.Collection {
	conn, err := db.Conn()
	if err == nil {
		return conn.Collection("containers")
	}
	return nil
}

func (s *S) getContainer(id string) (*container.Container, error) {
	coll := s.collection()
	defer coll.Close()
	var ret container.Container
	err := coll.Find(bson.M{"id": id}).One(&ret)
	return &ret, err
}

func (s *S) newClusterClient(c *check.C) *ClusterClient {
	cluster, err := cluster.New(nil, &cluster.MapStorage{}, "",
		cluster.Node{Address: s.server.URL()})
	c.Assert(err, check.IsNil)
	return &ClusterClient{
		Cluster:    cluster,
		Limiter:    &provision.LocalLimiter{},
		Collection: s.collection,
	}
}

func (s *S) TestSchedulerClientCreateContainerWithContainerCtx(c *check.C) {
	clusterClient := s.newClusterClient(c)
	ctx := context.WithValue(context.Background(), container.ContainerCtxKey{}, &container.Container{
		Container: types.Container{
			Name:    "mycont",
			AppName: "myapp",
			Image:   "localhost:5000/my/img",
			Status:  "building",
		},
	})
	cont, _, err := clusterClient.PullAndCreateContainer(docker.CreateContainerOptions{
		Name: "mycont",
		Config: &docker.Config{
			Image: "localhost:5000/my/img",
			Labels: map[string]string{
				"app-name": "myapp",
			},
		},
		Context: ctx,
	}, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := s.getContainer(cont.ID)
	c.Assert(err, check.IsNil)
	dbCont.MongoID = ""
	c.Assert(dbCont, check.DeepEquals, &container.Container{
		Container: types.Container{
			ID:       cont.ID,
			Name:     "mycont",
			AppName:  "myapp",
			Image:    "localhost:5000/my/img",
			HostAddr: "127.0.0.1",
			Status:   "building",
		},
	})
}

func (s *S) TestSchedulerClientCreateContainerNoContainerCtx(c *check.C) {
	client := s.newClusterClient(c)
	cont, _, err := client.PullAndCreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: "localhost:5000/my/img",
			Labels: map[string]string{
				"app-name": "myapp",
			},
		},
	}, nil)
	c.Assert(err, check.IsNil)
	_, err = s.getContainer(cont.ID)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
	cont, _, err = client.PullAndCreateContainer(docker.CreateContainerOptions{
		Name: "mycont",
		Config: &docker.Config{
			Image:  "localhost:5000/my/img",
			Labels: map[string]string{},
		},
	}, nil)
	c.Assert(err, check.IsNil)
	_, err = s.getContainer(cont.ID)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestSchedulerClientCreateContainerFailure(c *check.C) {
	s.server.PrepareFailure("myerr", "/containers/create")
	client := s.newClusterClient(c)
	_, _, err := client.PullAndCreateContainer(docker.CreateContainerOptions{
		Name: "mycont",
		Config: &docker.Config{
			Image: "localhost:5000/my/img",
			Labels: map[string]string{
				"app-name": "myapp",
			},
		},
	}, nil)
	c.Assert(err, check.ErrorMatches, `(?s).*myerr.*`)
	coll := s.collection()
	defer coll.Close()
	n, err := coll.Find(nil).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
}

func (s *S) TestSchedulerClientRemoveContainer(c *check.C) {
	client := s.newClusterClient(c)
	cont, _, err := client.PullAndCreateContainer(docker.CreateContainerOptions{
		Name: "mycont",
		Config: &docker.Config{
			Image: "localhost:5000/my/img",
			Labels: map[string]string{
				"app-name": "myapp",
			},
		},
	}, nil)
	c.Assert(err, check.IsNil)
	err = client.RemoveContainer(docker.RemoveContainerOptions{
		ID: cont.ID,
	})
	c.Assert(err, check.IsNil)
	_, err = s.getContainer(cont.ID)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}
