// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"net/url"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn      *db.Storage
	localhost *url.URL
}

var _ = check.Suite(&S{})

func init() {
	base := &S{}
	suite := &RouterSuite{
		SetUpSuiteFunc:   base.SetUpSuite,
		TearDownTestFunc: base.TearDownTest,
	}
	suite.SetUpTestFunc = func(c *check.C) {
		config.Set("database:name", "router_generic_fake_tests")
		base.SetUpTest(c)
		r := newFakeRouter()
		suite.Router = &r
	}
	check.Suite(suite)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_fake_tests")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake-hc:type", "fake-hc")
	s.localhost, _ = url.Parse("http://127.0.0.1")
}

func (s *S) SetUpTest(c *check.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Collection("router_fake_tests").Database)
}

func (s *S) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *S) TestShouldBeRegistered(c *check.C) {
	r, err := router.Get("fake")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.FitsTypeOf, &fakeRouter{})
	r, err = router.Get("fake-hc")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.FitsTypeOf, &hcRouter{})
}

func (s *S) TestAddBackend(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("foo")
	c.Assert(err, check.IsNil)
	defer r.RemoveBackend("foo")
	c.Assert(r.HasBackend("foo"), check.Equals, true)
}

func (s *S) TestAddDuplicateBackend(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("foo")
	c.Assert(err, check.IsNil)
	err = r.AddBackend("foo")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Backend already exists")
}

func (s *S) TestRemoveBackend(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("bar")
	c.Assert(err, check.IsNil)
	err = r.RemoveBackend("bar")
	c.Assert(err, check.IsNil)
	c.Assert(r.HasBackend("bar"), check.Equals, false)
}

func (s *S) TestRemoveUnknownBackend(c *check.C) {
	r := newFakeRouter()
	err := r.RemoveBackend("bar")
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *S) TestAddRoutes(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	c.Assert(r.HasRoute("name", s.localhost.String()), check.Equals, true)
}

func (s *S) TestAddRouteBackendNotFound(c *check.C) {
	r := newFakeRouter()
	err := r.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *S) TestRemoveRoutes(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	err = r.RemoveRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	c.Assert(r.HasRoute("name", s.localhost.String()), check.Equals, false)
}

func (s *S) TestRemoveRouteBackendNotFound(c *check.C) {
	r := newFakeRouter()
	err := r.RemoveRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *S) TestSetCName(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	err = r.SetCName("myapp.com", "name")
	c.Assert(err, check.IsNil)
	c.Assert(r.HasCName("myapp.com"), check.Equals, true)
	c.Assert(r.HasRoute("myapp.com", s.localhost.String()), check.Equals, true)
}

func (s *S) TestUnsetCName(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	err = r.SetCName("myapp.com", "name")
	c.Assert(err, check.IsNil)
	err = r.UnsetCName("myapp.com", "name")
	c.Assert(err, check.IsNil)
	c.Assert(r.HasCName("myapp.com"), check.Equals, false)
}

func (s *S) TestAddr(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	addr, err := r.Addr("name")
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "name.fakerouter.com")
	addr, err = r.Addr("unknown")
	c.Assert(addr, check.Equals, "")
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *S) TestAddrHCRouter(c *check.C) {
	r := newFakeRouter()
	hcr := hcRouter{fakeRouter: r}
	err := hcr.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = hcr.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	addr, err := hcr.Addr("name")
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "name.fakehcrouter.com")
	addr, err = hcr.Addr("unknown")
	c.Assert(addr, check.Equals, "")
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *S) TestReset(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	r.Reset()
	c.Assert(r.HasBackend("name"), check.Equals, false)
}

func (s *S) TestRoutes(c *check.C) {
	r := newFakeRouter()
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoutes("name", []*url.URL{s.localhost})
	c.Assert(err, check.IsNil)
	routes, err := r.Routes("name")
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{s.localhost})
}

func (s *S) TestSwap(c *check.C) {
	instance1 := s.localhost
	instance2, _ := url.Parse("http://127.0.0.2")
	backend1 := "b1"
	backend2 := "b2"
	r := newFakeRouter()
	err := r.AddBackend(backend1)
	c.Assert(err, check.IsNil)
	err = r.AddRoutes(backend1, []*url.URL{instance1})
	c.Assert(err, check.IsNil)
	err = r.AddBackend(backend2)
	c.Assert(err, check.IsNil)
	err = r.AddRoutes(backend2, []*url.URL{instance2})
	c.Assert(err, check.IsNil)
	retrieved1, err := router.Retrieve(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved1, check.Equals, backend1)
	retrieved2, err := router.Retrieve(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved2, check.Equals, backend2)
	err = r.Swap(backend1, backend2, false)
	c.Assert(err, check.IsNil)
	routes, err := r.Routes(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{instance2})
	routes, err = r.Routes(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{instance1})
	retrieved1, err = router.Retrieve(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved1, check.Equals, backend2)
	retrieved2, err = router.Retrieve(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved2, check.Equals, backend1)
	addr, err := r.Addr(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "b2.fakerouter.com")
	addr, err = r.Addr(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "b1.fakerouter.com")
}

func (s *S) TestAddCertificate(c *check.C) {
	r := TLSRouter
	err := r.AddCertificate("example.com", "cert", "key")
	c.Assert(err, check.IsNil)
	c.Assert(r.Certs["example.com"], check.DeepEquals, "cert")
	c.Assert(r.Keys["example.com"], check.DeepEquals, "key")
}

func (s *S) TestRemoveCertificate(c *check.C) {
	r := TLSRouter
	err := r.AddCertificate("example.com", "cert", "key")
	c.Assert(err, check.IsNil)
	err = r.RemoveCertificate("example.com")
	c.Assert(err, check.IsNil)
	c.Assert(r.Certs["example.com"], check.Equals, "")
	c.Assert(r.Keys["example.com"], check.Equals, "")
}

func (s *S) TestGetCertificate(c *check.C) {
	testCert := `-----BEGIN CERTIFICATE-----
MIIDkzCCAnugAwIBAgIJAIN09j/dhfmsMA0GCSqGSIb3DQEBCwUAMGAxCzAJBgNV
BAYTAkJSMRcwFQYDVQQIDA5SaW8gZGUgSmFuZWlybzEXMBUGA1UEBwwOUmlvIGRl
IEphbmVpcm8xDjAMBgNVBAoMBVRzdXJ1MQ8wDQYDVQQDDAZhcHAuaW8wHhcNMTcw
MTEyMjAzMzExWhcNMjcwMTEwMjAzMzExWjBgMQswCQYDVQQGEwJCUjEXMBUGA1UE
CAwOUmlvIGRlIEphbmVpcm8xFzAVBgNVBAcMDlJpbyBkZSBKYW5laXJvMQ4wDAYD
VQQKDAVUc3VydTEPMA0GA1UEAwwGYXBwLmlvMIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEAw3GRuXOyL0Ar5BYA8DAPkY7ZHtHpEFK5bOoZB3lLBMjIbUKk
+riNTTgcY1eCsoAMZ0ZGmwmK/8mrJSBcsK/f1HVTcsSU0pA961ROPkAad/X/luSL
nXxDnZ1c0cOeU3GC4limB4CSZ64SZEDJvkUWnhUjTO4jfOCu0brkEnF8x3fpxfAy
OrAO50Uxij3VOQIAkP5B0T6x2Htr1ogm/vuubp5IG+KVuJHbozoaFFgRnDwrk+3W
k3FFUvg4ywY2jgJMLFJb0U3IIQgSqwQwXftKdu1EaoxA5fQmu/3a4CvYKKkwLJJ+
6L4O9Uf+QgaBZqTpDJ7XcIYbW+TPffzSwuI5PwIDAQABo1AwTjAdBgNVHQ4EFgQU
3XOK6bQW7hL47fMYH8JT/qCqIDgwHwYDVR0jBBgwFoAU3XOK6bQW7hL47fMYH8JT
/qCqIDgwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAgP4K9Zd1xSOQ
HAC6p2XjuveBI9Aswudaqg8ewYZtbtcbV70db+A69b8alSXfqNVqI4L2T97x/g6J
8ef8MG6TExhd1QktqtxtR+wsiijfUkityj8j5JT36TX3Kj0eIXrLJWxPEBhtGL17
ZBGdNK2/tDsQl5Wb+qnz5Ge9obybRLHHL2L5mrSwb+nC+nrC2nlfjJgVse9HhU9j
6Euq5hstXAlQH7fUbC5zAMS5UFrbzR+hOvjrSwzkkJmKW8BKKCfSaevRhq4VXxpw
Wx1oQV8UD5KLQQRy9Xew/KRHVzOpdkK66/i/hgV7GdREy4aKNAEBRpheOzjLDQyG
YRLI1QVj1Q==
-----END CERTIFICATE-----`
	r := TLSRouter
	err := r.AddCertificate("example.com", testCert, "key")
	c.Assert(err, check.IsNil)
	cert, err := r.GetCertificate("example.com")
	c.Assert(err, check.IsNil)
	c.Assert(cert, check.DeepEquals, testCert)
}

func (s *S) TestAddBackendOpts(c *check.C) {
	r := OptsRouter
	err := r.AddBackendOpts("myapp", map[string]string{"opt1": "val1"})
	c.Assert(err, check.IsNil)
	c.Assert(r.Opts, check.DeepEquals, map[string]string{"opt1": "val1"})
}

func (s *S) TestUpdateBackendOpts(c *check.C) {
	r := OptsRouter
	err := r.UpdateBackendOpts("myapp", map[string]string{"opt1": "val1"})
	c.Assert(err, check.IsNil)
	c.Assert(r.Opts, check.DeepEquals, map[string]string{"opt1": "val1"})
}
