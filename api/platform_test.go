// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

type PlatformSuite struct{}

var _ = gocheck.Suite(&PlatformSuite{})

func (p *PlatformSuite) TestPlatformAdd(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	dockerfile_url := "http://localhost/Dockerfile"
	body := fmt.Sprintf("name=%s&dockerfile=%s", "teste", dockerfile_url)
	request, _ := http.NewRequest("POST", "/platforms/add", strings.NewReader(body))
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	result := platformAdd(recorder, request, nil)
	c.Assert(result, gocheck.IsNil)
}

func (p *PlatformSuite) TestPlatformUpdate(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd("wat", nil, nil)
	c.Assert(err, gocheck.IsNil)
	dockerfile_url := "http://localhost/Dockerfile"
	body := fmt.Sprintf("dockerfile=%s", dockerfile_url)
	request, _ := http.NewRequest("PUT", "/platforms/wat?:name=wat", strings.NewReader(body))
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	result := platformUpdate(recorder, request, nil)
	c.Assert(result, gocheck.IsNil)
}

func (p *PlatformSuite) TestPlatformRemove(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd("test", nil, nil)
	c.Assert(err, gocheck.IsNil)
	request, _ := http.NewRequest("DELETE", "/platforms/test?:name=test", nil)
	recorder := httptest.NewRecorder()
	err = platformRemove(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
}
