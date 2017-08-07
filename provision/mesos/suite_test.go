// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import (
	"math/rand"
	"testing"

	"github.com/andygrunwald/megos"
	"github.com/gambol99/go-marathon"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/storage"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

type S struct {
	p        *mesosProvisioner
	conn     *db.Storage
	user     *auth.User
	team     *auth.Team
	token    auth.Token
	marathon *fakeMarathonClient
	mesos    *fakeMesosClient
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("kubernetes:token", "token==")
	config.Set("database:name", "provision_mesos_tests_s")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

type fakeMarathonClient struct {
	marathon.Marathon
}

type fakeMesosClient struct {
	mesosClient
	state megos.State
}

func (c *fakeMesosClient) GetSlavesFromCluster() (*megos.State, error) {
	return &c.state, nil
}

func (s *S) addFakeNodes() {
	s.mesos.state = megos.State{
		Slaves: []megos.Slave{
			{
				ID:       "m1id",
				Hostname: "m1",
			},
			{
				ID:       "m2id",
				Hostname: "m2",
			},
		},
	}
}

func (s *S) SetUpTest(c *check.C) {
	s.marathon = &fakeMarathonClient{}
	s.mesos = &fakeMesosClient{}
	hookMarathonClient = func(cli marathon.Marathon) marathon.Marathon {
		s.marathon.Marathon = cli
		return s.marathon
	}
	hookMesosClient = func(cli mesosClient) mesosClient {
		s.mesos.mesosClient = cli
		return s.mesos
	}
	routertest.FakeRouter.Reset()
	rand.Seed(0)
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	clus := &cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"http://addr1"},
		Default:     true,
		Provisioner: provisionerName,
	}
	err = clus.Save()
	c.Assert(err, check.IsNil)
	err = provision.AddPool(provision.AddPoolOptions{
		Name:        "bonehunters",
		Default:     true,
		Provisioner: "mesos",
	})
	c.Assert(err, check.IsNil)
	p := app.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	err = p.Save()
	c.Assert(err, check.IsNil)
	s.p = &mesosProvisioner{}
	s.user = &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "admin"}
	c.Assert(err, check.IsNil)
	err = storage.TeamRepository.Insert(storage.Team(*s.team))
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}
