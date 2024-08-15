// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	routers map[string]routerFactory
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "router_tests")
	var err error
	storagev2.Reset()
	servicemanager.DynamicRouter, err = DynamicRouterService()
	c.Assert(err, check.IsNil)

}
func (s *S) SetUpTest(c *check.C) {
	s.routers = make(map[string]routerFactory)
	for k, v := range routers {
		s.routers[k] = v
	}
	storagev2.ClearAllCollections(nil)
}

func (s *S) TearDownTest(c *check.C) {
	routers = s.routers
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}
