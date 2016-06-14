// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/check.v1"
)

type PlatformSuite struct {
	conn *db.Storage
}

var _ = check.Suite(&PlatformSuite{})

func createToken(c *check.C) auth.Token {
	user := &auth.User{Email: "platform-admin" + "@groundcontrol.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme.Remove(user)
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole("platform-admin", string(permission.CtxGlobal), "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("*")
	c.Assert(err, check.IsNil)
	err = user.AddRole(role.Name, "")
	c.Assert(err, check.IsNil)
	return token
}

func (s *PlatformSuite) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_platform_test")
	var err error
	app.AuthScheme = nativeScheme
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *PlatformSuite) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *PlatformSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *PlatformSuite) TestPlatformAdd(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	var buf bytes.Buffer
	dockerfileURL := "http://localhost/Dockerfile"
	writer := multipart.NewWriter(&buf)
	writer.WriteField("name", "test")
	writer.WriteField("dockerfile", dockerfileURL)
	fileWriter, err := writer.CreateFormFile("dockerfile_content", "Dockerfile")
	c.Assert(err, check.IsNil)
	fileWriter.Write([]byte("FROM tsuru/java"))
	writer.Close()
	request, _ := http.NewRequest("POST", "/platforms/add", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	token := createToken(c)
	result := platformAdd(recorder, request, token)
	c.Assert(result, check.IsNil)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
}

func (s *PlatformSuite) TestPlatformUpdate(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd(provision.PlatformOptions{Name: "wat", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	dockerfileURL := "http://localhost/Dockerfile"
	writer := multipart.NewWriter(&buf)
	writer.WriteField("dockerfile", dockerfileURL)
	writer.Close()
	request, _ := http.NewRequest("PUT", "/platforms/wat", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
}

func (s *PlatformSuite) TestPlatformUpdateOnlyDisableTrue(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd(provision.PlatformOptions{Name: "wat", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	dockerfileURL := ""
	body := fmt.Sprintf("dockerfile=%s", dockerfileURL)
	request, err := http.NewRequest("PUT", "/platforms/wat?disabled=true", strings.NewReader(body))
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Add("Authorization", "b "+token.GetValue())
	request.Header.Add("Content-Type", "multipart/form-data")
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueAndDockerfile(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd(provision.PlatformOptions{Name: "wat", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	dockerfileURL := "http://localhost/Dockerfile"
	body := fmt.Sprintf("dockerfile=%s", dockerfileURL)
	request, _ := http.NewRequest("PUT", "/platforms/wat?:name=wat&disabled=true", strings.NewReader(body))
	request.Header.Add("Content-Type", "multipart/form-data")
	recorder := httptest.NewRecorder()
	token := createToken(c)
	result := platformUpdate(recorder, request, token)
	c.Assert(result, check.IsNil)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
}

func (s *PlatformSuite) TestPlatformUpdateOnlyDisableFalse(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd(provision.PlatformOptions{Name: "wat", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	dockerfileURL := ""
	body := fmt.Sprintf("dockerfile=%s", dockerfileURL)
	request, _ := http.NewRequest("PUT", "/platforms/wat?:name=wat&disabled=false", strings.NewReader(body))
	request.Header.Add("Content-Type", "multipart/form-data")
	recorder := httptest.NewRecorder()
	token := createToken(c)
	result := platformUpdate(recorder, request, token)
	c.Assert(result, check.IsNil)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
}

func (s *PlatformSuite) TestPlatformUpdateDisableFalseAndDockerfile(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd(provision.PlatformOptions{Name: "wat", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	dockerfileURL := "http://localhost/Dockerfile"
	body := fmt.Sprintf("dockerfile=%s", dockerfileURL)
	request, _ := http.NewRequest("PUT", "/platforms/wat?:name=wat&disabled=false", strings.NewReader(body))
	request.Header.Add("Content-Type", "multipart/form-data")
	recorder := httptest.NewRecorder()
	token := createToken(c)
	result := platformUpdate(recorder, request, token)
	c.Assert(result, check.IsNil)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
}

func (*PlatformSuite) TestPlatformUpdateNotFound(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	var buf bytes.Buffer
	dockerfileURL := "http://localhost/Dockerfile"
	writer := multipart.NewWriter(&buf)
	writer.WriteField("dockerfile", dockerfileURL)
	writer.Close()
	request, _ := http.NewRequest("PUT", "/platforms/not-found", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (*PlatformSuite) TestPlatformRemoveNotFound(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	request, err := http.NewRequest("DELETE", "/platforms/not-found", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (*PlatformSuite) TestPlatformRemove(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	oldProvisioner := app.Provisioner
	app.Provisioner = &provisioner
	defer func() {
		app.Provisioner = oldProvisioner
	}()
	err := app.PlatformAdd(provision.PlatformOptions{Name: "test", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	request, _ := http.NewRequest("DELETE", "/platforms/test?:name=test", nil)
	recorder := httptest.NewRecorder()
	token := createToken(c)
	err = platformRemove(recorder, request, token)
	c.Assert(err, check.IsNil)
}

func (s *PlatformSuite) TestPlatformList(c *check.C) {
	platforms := []app.Platform{
		{Name: "python"},
		{Name: "java"},
		{Name: "ruby20"},
		{Name: "static"},
	}
	for _, p := range platforms {
		s.conn.Platforms().Insert(p)
		defer s.conn.Platforms().Remove(p)
	}
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got []app.Platform
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, platforms)
}

func (s *PlatformSuite) TestPlatformListGetDisabledPlatforms(c *check.C) {
	platforms := []app.Platform{
		{Name: "python", Disabled: true},
		{Name: "java"},
		{Name: "ruby20", Disabled: true},
		{Name: "static"},
	}
	for _, p := range platforms {
		s.conn.Platforms().Insert(p)
		defer s.conn.Platforms().Remove(p)
	}
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got []app.Platform
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, platforms)
}

func (s *PlatformSuite) TestPlatformListUserList(c *check.C) {
	platforms := []app.Platform{
		{Name: "python", Disabled: true},
		{Name: "java", Disabled: false},
		{Name: "ruby20", Disabled: true},
		{Name: "static"},
	}
	expectedPlatforms := []app.Platform{
		{Name: "java", Disabled: false},
		{Name: "static"},
	}
	for _, p := range platforms {
		s.conn.Platforms().Insert(p)
		defer s.conn.Platforms().Remove(p)
	}
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got []app.Platform
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expectedPlatforms)
}

func (s *PlatformSuite) TestPlatformListNoContent(c *check.C) {
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}
