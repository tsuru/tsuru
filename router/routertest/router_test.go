// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"context"
	"net/url"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/router"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	localhost *url.URL
}

var _ = check.Suite(&S{})

func init() {
	setupGenericSuite(func(c *check.C, base *S, suite *RouterSuite) {
		r := newFakeRouter()
		suite.Router = &r
	})
}

func setupGenericSuite(fn func(c *check.C, base *S, suite *RouterSuite)) {
	base := &S{}
	suite := &RouterSuite{
		SetUpSuiteFunc:   base.SetUpSuite,
		TearDownTestFunc: base.TearDownTest,
	}
	suite.SetUpTestFunc = func(c *check.C) {
		config.Set("database:name", "router_generic_fake_tests")
		base.SetUpTest(c)
		fn(c, base, suite)
	}
	check.Suite(suite)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "router_fake_tests")
	config.Set("routers:fake:type", "fake")
	s.localhost, _ = url.Parse("http://127.0.0.1")

	storagev2.Reset()
}

func (s *S) SetUpTest(c *check.C) {
	storagev2.ClearAllCollections(nil)
	servicemock.SetMockService(&servicemock.MockService{})
}

func (s *S) TearDownTest(c *check.C) {
}

func (s *S) TestEnsureBackend(c *check.C) {
	r := newFakeRouter()
	app := FakeApp{Name: "foo"}
	err := r.EnsureBackend(context.TODO(), app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	defer r.RemoveBackend(context.TODO(), app)
	c.Assert(r.HasBackend("foo"), check.Equals, true)
}

func (s *S) TestRemoveBackend(c *check.C) {
	r := newFakeRouter()
	app := FakeApp{Name: "bar"}
	err := r.EnsureBackend(context.TODO(), app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	err = r.RemoveBackend(context.TODO(), app)
	c.Assert(err, check.IsNil)
	c.Assert(r.HasBackend("bar"), check.Equals, false)
}

func (s *S) TestRemoveUnknownBackend(c *check.C) {
	r := newFakeRouter()
	app := FakeApp{Name: "bar"}
	err := r.RemoveBackend(context.TODO(), app)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *S) TestReset(c *check.C) {
	r := newFakeRouter()
	app := FakeApp{Name: "name"}
	err := r.EnsureBackend(context.TODO(), app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	r.Reset()
	c.Assert(r.HasBackend("name"), check.Equals, false)
}

func (s *S) TestAddCertificate(c *check.C) {
	r := TLSRouter
	err := r.AddCertificate(context.TODO(), FakeApp{Name: "myapp"}, "example.com", "cert", "key")
	c.Assert(err, check.IsNil)
	c.Assert(r.Certs["example.com"], check.DeepEquals, "cert")
	c.Assert(r.Keys["example.com"], check.DeepEquals, "key")
}

func (s *S) TestRemoveCertificate(c *check.C) {
	r := TLSRouter
	err := r.AddCertificate(context.TODO(), FakeApp{Name: "myapp"}, "example.com", "cert", "key")
	c.Assert(err, check.IsNil)
	err = r.RemoveCertificate(context.TODO(), FakeApp{Name: "myapp"}, "example.com")
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
	err := r.AddCertificate(context.TODO(), FakeApp{Name: "myapp"}, "example.com", testCert, "key")
	c.Assert(err, check.IsNil)
	cert, err := r.GetCertificate(context.TODO(), FakeApp{Name: "myapp"}, "example.com")
	c.Assert(err, check.IsNil)
	c.Assert(cert, check.DeepEquals, testCert)
}
