// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("auth:oauth:client-id", "clientid")
	config.Set("auth:oauth:client-secret", "clientsecret")
	config.Set("auth:oauth:scope", "myscope")
	config.Set("auth:oauth:auth-url", "http://url1.com")
	config.Set("auth:oauth:token-url", "http://url2.com")
	config.Set("auth:oauth:info-url", "http://url3.com")
	config.Set("admin-team", "admin")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_auth_native_test")
}
