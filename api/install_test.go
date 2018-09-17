// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/tsuru/install"
	"github.com/tsuru/tsuru/permission"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestInstallHostAdd(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermInstallManage,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`name=xyz&driverName=amazonec2&driver={"SSHPort": 22}`)
	request, err := http.NewRequest("POST", "/install/hosts", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	defer s.conn.InstallHosts().RemoveAll(nil)
	var hosts []install.Host
	err = s.conn.InstallHosts().Find(nil).All(&hosts)
	c.Assert(err, check.IsNil)
	c.Assert(len(hosts), check.Equals, 1)
	c.Assert(hosts[0].Name, check.Equals, "xyz")
	c.Assert(hosts[0].DriverName, check.Equals, "amazonec2")
	c.Assert(hosts[0].Driver, check.DeepEquals, map[string]interface{}{"SSHPort": float64(22)})
}

func (s *S) TestInstallHostReturnsForbiddenIfNoPermissions(c *check.C) {
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`name=xyz&driverName=amazonec2&driver={"IP": "127.0.0.1"}`)
	request, err := http.NewRequest("POST", "/install/hosts", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
	var hosts []install.Host
	err = s.conn.InstallHosts().Find(nil).All(&hosts)
	c.Assert(err, check.IsNil)
	c.Assert(hosts, check.IsNil)
}

func (s *S) TestInstallHostInfo(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermInstallManage,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	expectedHost := &install.Host{Name: "my-host", DriverName: "amazonec2", Driver: make(map[string]interface{})}
	err := install.AddHost(expectedHost)
	c.Assert(err, check.IsNil)
	defer s.conn.InstallHosts().RemoveAll(nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/install/hosts/my-host", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	host := &install.Host{}
	err = json.Unmarshal(recorder.Body.Bytes(), &host)
	c.Assert(err, check.IsNil)
	c.Assert(host, check.DeepEquals, expectedHost)
}

func (s *S) TestInstallHostInfoReturnsForbiddenIfNoPermissions(c *check.C) {
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/install/hosts/my-host", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestInstallHostInfoReturnsNotFoundWhenHostDoesNotExist(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermInstallManage,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/install/hosts/unknown-host", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Host unknown-host not found.\n")
}

func (s *S) TestInstallHostList(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermInstallManage,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	host1 := &install.Host{Name: "my-host-1", DriverName: "amazonec2", Driver: make(map[string]interface{})}
	err := install.AddHost(host1)
	c.Assert(err, check.IsNil)
	host2 := &install.Host{Name: "my-host-2", DriverName: "amazonec2", Driver: make(map[string]interface{})}
	err = install.AddHost(host2)
	c.Assert(err, check.IsNil)
	defer s.conn.InstallHosts().RemoveAll(nil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/install/hosts", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	hosts := []install.Host{}
	err = json.Unmarshal(recorder.Body.Bytes(), &hosts)
	c.Assert(err, check.IsNil)
	c.Assert(hosts, check.DeepEquals, []install.Host{*host1, *host2})
}

func (s *S) TestInstallHostListReturnsForbiddenIfNoPermissions(c *check.C) {
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/install/hosts", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}
