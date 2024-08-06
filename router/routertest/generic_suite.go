// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"context"
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/router"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	check "gopkg.in/check.v1"
)

type FakeApp struct {
	Name      string
	Pool      string
	Teams     []string
	TeamOwner string
}

func (r FakeApp) GetName() string {
	return r.Name
}

func (r FakeApp) GetPool() string {
	return r.Pool
}

func (r FakeApp) GetTeamOwner() string {
	return r.TeamOwner
}

func (r FakeApp) GetTeamsName() []string {
	return r.Teams
}

type RouterSuite struct {
	Router            router.Router
	SetUpSuiteFunc    func(c *check.C)
	SetUpTestFunc     func(c *check.C)
	TearDownSuiteFunc func(c *check.C)
	TearDownTestFunc  func(c *check.C)

	ctx context.Context
}

func (s *RouterSuite) SetUpSuite(c *check.C) {
	s.ctx = context.Background()
	if s.SetUpSuiteFunc != nil {
		s.SetUpSuiteFunc(c)
	}
}

func (s *RouterSuite) SetUpTest(c *check.C) {
	if s.SetUpTestFunc != nil {
		s.SetUpTestFunc(c)
	}
	servicemock.SetMockService(&servicemock.MockService{})
	c.Logf("generic router test for %T", s.Router)
}

func (s *RouterSuite) TearDownSuite(c *check.C) {
	if s.TearDownSuiteFunc != nil {
		s.TearDownSuiteFunc(c)
	}
	if _, err := config.GetString("database:name"); err == nil {
		conn, err := db.Conn()
		c.Assert(err, check.IsNil)
		defer conn.Close()
		storagev2.ClearAllCollections(nil)
	}
}

func (s *RouterSuite) TearDownTest(c *check.C) {
	if s.TearDownTestFunc != nil {
		s.TearDownTestFunc(c)
	}
}

type URLList []*url.URL

func (l URLList) Len() int           { return len(l) }
func (l URLList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l URLList) Less(i, j int) bool { return l[i].Host < l[j].Host }

func (s *RouterSuite) TestGetInfo(c *check.C) {
	msg, err := s.Router.GetInfo(s.ctx)
	c.Assert(err, check.IsNil)
	c.Assert(msg, check.NotNil)
}
