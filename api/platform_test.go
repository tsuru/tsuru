// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

type PlatformSuite struct{}

var _ = check.Suite(&PlatformSuite{})

func (s *PlatformSuite) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_platform_test")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (p *PlatformSuite) TestPlatformAdd(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
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
	c.Assert(result, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, "\nOK!\n")
}

func (p *PlatformSuite) TestPlatformUpdate(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd("wat", nil, nil)
	c.Assert(err, check.IsNil)
	dockerfile_url := "http://localhost/Dockerfile"
	body := fmt.Sprintf("dockerfile=%s", dockerfile_url)
	request, _ := http.NewRequest("PUT", "/platforms/wat?:name=wat", strings.NewReader(body))
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	result := platformUpdate(recorder, request, nil)
	c.Assert(result, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, "\nOK!\n")
}

func (p *PlatformSuite) TestPlatformRemove(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd("test", nil, nil)
	c.Assert(err, check.IsNil)
	request, _ := http.NewRequest("DELETE", "/platforms/test?:name=test", nil)
	recorder := httptest.NewRecorder()
	err = platformRemove(recorder, request, nil)
	c.Assert(err, check.IsNil)
}
