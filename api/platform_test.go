// Copyright 2014 tsuru authors. All rights reserved.
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
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/builder/fake"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository/repositorytest"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

type PlatformSuite struct {
	conn       *db.Storage
	builder    *fake.FakeBuilder
	testServer http.Handler
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

func (s *PlatformSuite) reset() {
	s.builder.Reset()
	repositorytest.Reset()
}

func (s *PlatformSuite) SetUpSuite(c *check.C) {
	s.builder = fake.NewFakeBuilder()
	builder.Register("fake", s.builder)
	s.testServer = RunServer(true)
}

func (s *PlatformSuite) SetUpTest(c *check.C) {
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_platform_test")
	var err error
	app.AuthScheme = nativeScheme
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	provision.DefaultProvisioner = "fake-extensible"
	builder.DefaultBuilder = "fake"
	s.reset()
}

func (s *PlatformSuite) TearDownTest(c *check.C) {
	s.reset()
	s.conn.Close()
}

func (s *PlatformSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *PlatformSuite) TestPlatformAdd(c *check.C) {
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
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlatform, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "platform.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "test"},
			{"name": "dockerfile", "value": dockerfileURL},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformAddInvalidName(c *check.C) {
	var buf bytes.Buffer
	dockerfileURL := "http://localhost/Dockerfile"
	writer := multipart.NewWriter(&buf)
	writer.WriteField("name", "Invalid_Name")
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
	c.Assert(result, check.DeepEquals, appTypes.ErrInvalidPlatformName)
}

func (s *PlatformSuite) TestPlatformUpdate(c *check.C) {
	err := app.PlatformAdd(builder.PlatformOptions{Name: "wat", Args: nil, Output: nil})
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
	s.testServer.ServeHTTP(recorder, request)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlatform, Value: "wat"},
		Owner:  token.GetUserName(),
		Kind:   "platform.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "wat"},
			{"name": "dockerfile", "value": dockerfileURL},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformUpdateOnlyDisableTrue(c *check.C) {
	err := app.PlatformAdd(builder.PlatformOptions{Name: "wat", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("dockerfile", "")
	writer.WriteField("disabled", "true")
	writer.Close()
	request, err := http.NewRequest("PUT", "/platforms/wat", &buf)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Add("Authorization", "b "+token.GetValue())
	request.Header.Add("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlatform, Value: "wat"},
		Owner:  token.GetUserName(),
		Kind:   "platform.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "wat"},
			{"name": "disabled", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueAndDockerfile(c *check.C) {
	err := app.PlatformAdd(builder.PlatformOptions{Name: "wat", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	dockerfileURL := "http://localhost/Dockerfile"
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("dockerfile", dockerfileURL)
	writer.WriteField("disabled", "true")
	writer.Close()
	request, _ := http.NewRequest("PUT", "/platforms/wat?:name=wat", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	token := createToken(c)
	result := platformUpdate(recorder, request, token)
	c.Assert(result, check.IsNil)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlatform, Value: "wat"},
		Owner:  token.GetUserName(),
		Kind:   "platform.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "wat"},
			{"name": "disabled", "value": "true"},
			{"name": "dockerfile", "value": dockerfileURL},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformUpdateOnlyDisableFalse(c *check.C) {
	err := app.PlatformAdd(builder.PlatformOptions{Name: "wat", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	dockerfileURL := ""
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("dockerfile", dockerfileURL)
	writer.WriteField("disabled", "false")
	writer.Close()
	request, _ := http.NewRequest("PUT", "/platforms/wat?:name=wat", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	token := createToken(c)
	result := platformUpdate(recorder, request, token)
	c.Assert(result, check.IsNil)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(msg.Error, check.Equals, "")
}

func (s *PlatformSuite) TestPlatformUpdateDisableFalseAndDockerfile(c *check.C) {
	err := app.PlatformAdd(builder.PlatformOptions{Name: "wat", Args: nil, Output: nil})
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

func (s *PlatformSuite) TestPlatformUpdateNotFound(c *check.C) {
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
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *PlatformSuite) TestPlatformRemoveNotFound(c *check.C) {
	request, err := http.NewRequest("DELETE", "/platforms/not-found", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (*PlatformSuite) TestPlatformRemove(c *check.C) {
	err := app.PlatformAdd(builder.PlatformOptions{Name: "test", Args: nil, Output: nil})
	c.Assert(err, check.IsNil)
	request, _ := http.NewRequest("DELETE", "/platforms/test?:name=test", nil)
	recorder := httptest.NewRecorder()
	token := createToken(c)
	err = platformRemove(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlatform, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "platform.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "test"},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformList(c *check.C) {
	platforms := []appTypes.Platform{
		{Name: "python"},
		{Name: "java"},
		{Name: "ruby20"},
		{Name: "static"},
	}
	for _, p := range platforms {
		app.PlatformService().Insert(p)
	}
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got []appTypes.Platform
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, platforms)
}

func (s *PlatformSuite) TestPlatformListGetDisabledPlatforms(c *check.C) {
	platforms := []appTypes.Platform{
		{Name: "python", Disabled: true},
		{Name: "java"},
		{Name: "ruby20", Disabled: true},
		{Name: "static"},
	}
	for _, p := range platforms {
		app.PlatformService().Insert(p)
	}
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got []appTypes.Platform
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, platforms)
}

func (s *PlatformSuite) TestPlatformListUserList(c *check.C) {
	platforms := []appTypes.Platform{
		{Name: "python", Disabled: true},
		{Name: "java", Disabled: false},
		{Name: "ruby20", Disabled: true},
		{Name: "static"},
	}
	expectedPlatforms := []appTypes.Platform{
		{Name: "java", Disabled: false},
		{Name: "static"},
	}
	for _, p := range platforms {
		app.PlatformService().Insert(p)
	}
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got []appTypes.Platform
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
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}
