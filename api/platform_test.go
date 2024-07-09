// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	appTypes "github.com/tsuru/tsuru/types/app"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

type PlatformSuite struct {
	conn        *db.Storage
	testServer  http.Handler
	mockService servicemock.MockService
}

var _ = check.Suite(&PlatformSuite{})

func createToken(c *check.C) auth.Token {
	user := &auth.User{Email: "platform-admin" + "@groundcontrol.com", Password: "123456", Quota: quota.UnlimitedQuota}
	nativeScheme.Remove(context.TODO(), user)
	_, err := nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole("platform-admin", string(permTypes.CtxGlobal), "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(context.TODO(), "*")
	c.Assert(err, check.IsNil)
	err = user.AddRole(context.TODO(), role.Name, "")
	c.Assert(err, check.IsNil)
	return token
}

func (s *PlatformSuite) SetUpSuite(c *check.C) {
	s.testServer = RunServer(true)
}

func (s *PlatformSuite) SetUpTest(c *check.C) {
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_api_platform_test")
	storagev2.Reset()
	var err error
	app.AuthScheme = nativeScheme
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	provision.DefaultProvisioner = "fake-extensible"
	servicemock.SetMockService(&s.mockService)
}

func (s *PlatformSuite) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *PlatformSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
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
	request, _ := http.NewRequest("POST", "/platforms", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePlatform, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "platform.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "test"},
			{"name": "dockerfile", "value": dockerfileURL},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformAddError(c *check.C) {
	name := "Invalid_Name"
	dockerfileURL := "http://localhost/Dockerfile"
	createErr := errors.New("something wrong happened")
	s.mockService.Platform.OnCreate = func(opts appTypes.PlatformOptions) error {
		c.Assert(opts.Args["name"], check.Equals, name)
		c.Assert(opts.Args["dockerfile"], check.Equals, dockerfileURL)
		c.Assert(opts.Name, check.Equals, name)
		return createErr
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("name", name)
	writer.WriteField("dockerfile", dockerfileURL)
	fileWriter, err := writer.CreateFormFile("dockerfile_content", "Dockerfile")
	c.Assert(err, check.IsNil)
	fileWriter.Write([]byte("FROM tsuru/java"))
	writer.Close()
	request, _ := http.NewRequest("POST", "/platforms", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.DeepEquals, createErr.Error()+"\n")
}

func (s *PlatformSuite) TestPlatformAddMissingFile(c *check.C) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("name", "test")
	writer.Close()
	request, _ := http.NewRequest("POST", "/platforms", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "http: no such file\n")
}

func (s *PlatformSuite) TestPlatformAddMissingFileContent(c *check.C) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("name", "test")
	_, err := writer.CreateFormFile("dockerfile_content", "Dockerfile")
	c.Assert(err, check.IsNil)
	writer.Close()
	request, _ := http.NewRequest("POST", "/platforms", &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, appTypes.ErrMissingFileContent.Error()+"\n")
}

func (s *PlatformSuite) TestPlatformUpdate(c *check.C) {
	platformName := "wat"
	s.mockService.Platform.OnUpdate = func(opts appTypes.PlatformOptions) error {
		c.Assert(opts.Data, check.DeepEquals, []byte(`FROM tsuru/scratch:latest`))
		c.Assert(opts.Name, check.Equals, platformName)
		return nil
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	ww, err := writer.CreateFormFile("dockerfile_content", "Dockerfile")
	c.Assert(err, check.IsNil)
	_, err = ww.Write([]byte(`FROM tsuru/scratch:latest`))
	c.Assert(err, check.IsNil)
	writer.Close()
	request, err := http.NewRequest("PUT", "/platforms/wat", &buf)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.GetValue()))
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var msg io.SimpleJsonMessage
	err = json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(err, check.IsNil)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePlatform, Value: platformName},
		Owner:  token.GetUserName(),
		Kind:   "platform.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": platformName},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformUpdateOnlyDisableTrue(c *check.C) {
	platformName := "wat"
	s.mockService.Platform.OnUpdate = func(opts appTypes.PlatformOptions) error {
		c.Assert(opts.Data, check.HasLen, 0)
		c.Assert(opts.Args["disabled"], check.Equals, "true")
		c.Assert(opts.Name, check.Equals, platformName)
		return nil
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("disabled", "true")
	writer.Close()
	request, err := http.NewRequest("PUT", fmt.Sprintf("/platforms/%s", platformName), &buf)
	c.Assert(err, check.IsNil)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.GetValue()))
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePlatform, Value: platformName},
		Owner:  token.GetUserName(),
		Kind:   "platform.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": platformName},
			{"name": "disabled", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueAndDockerfile(c *check.C) {
	platformName := "wat"
	s.mockService.Platform.OnUpdate = func(opts appTypes.PlatformOptions) error {
		c.Assert(opts.Data, check.DeepEquals, []byte(`FROM scratch`))
		c.Assert(opts.Args["disabled"], check.Equals, "true")
		c.Assert(opts.Name, check.Equals, platformName)
		return nil
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	ww, err := writer.CreateFormFile("dockerfile_content", "Dockerfile")
	c.Assert(err, check.IsNil)
	_, err = ww.Write([]byte(`FROM scratch`))
	c.Assert(err, check.IsNil)
	writer.WriteField("disabled", "true")
	writer.Close()
	request, _ := http.NewRequest("PUT", fmt.Sprintf("/platforms/%s", platformName), &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.GetValue()))
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePlatform, Value: platformName},
		Owner:  token.GetUserName(),
		Kind:   "platform.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": platformName},
			{"name": "disabled", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformUpdate_WhenReturnsAnError(c *check.C) {
	name := "not-found"
	s.mockService.Platform.OnUpdate = func(opts appTypes.PlatformOptions) error {
		return appTypes.ErrPlatformNotFound
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("disabled", "true")
	writer.Close()
	request, _ := http.NewRequest("PUT", fmt.Sprintf("/platforms/%s", name), &buf)
	request.Header.Add("Content-Type", writer.FormDataContentType())
	token := createToken(c)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.GetValue()))
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *PlatformSuite) TestPlatformRemoveNotFound(c *check.C) {
	name := "not-found"
	s.mockService.Platform.OnRemove = func(n string) error {
		c.Assert(n, check.Equals, name)
		return appTypes.ErrPlatformNotFound
	}
	request, err := http.NewRequest("DELETE", "/platforms/"+name, nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *PlatformSuite) TestPlatformRemove(c *check.C) {
	name := "test"
	s.mockService.Platform.OnRemove = func(n string) error {
		c.Assert(n, check.Equals, name)
		return nil
	}
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/platforms/%s?:name=%s", name, name), nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePlatform, Value: name},
		Owner:  token.GetUserName(),
		Kind:   "platform.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": name},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformList(c *check.C) {
	platforms := []appTypes.Platform{
		{Name: "java"},
		{Name: "static", Disabled: true},
	}
	s.mockService.Platform.OnList = func(enabledOnly bool) ([]appTypes.Platform, error) {
		c.Assert(enabledOnly, check.Equals, false)
		return platforms, nil
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

func (s *PlatformSuite) TestPlatformListGetOnlyEnabledPlatforms(c *check.C) {
	platforms := []appTypes.Platform{
		{Name: "python"},
		{Name: "ruby"},
	}
	s.mockService.Platform.OnList = func(enabledOnly bool) ([]appTypes.Platform, error) {
		c.Assert(enabledOnly, check.Equals, true)
		return platforms, nil
	}
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxGlobal, ""),
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

func (s *PlatformSuite) TestPlatformListNoContent(c *check.C) {
	request, err := http.NewRequest("GET", "/platforms", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *PlatformSuite) TestPlatformInfo(c *check.C) {
	type result struct {
		Platform appTypes.Platform
		Images   []string
	}
	expected := result{
		Platform: appTypes.Platform{Name: "myPlatform"},
		Images:   []string{"tsuru/myplatform:v1", "tsuru/myplatform:v2"},
	}
	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		c.Assert(name, check.Equals, "myplatform")
		return &expected.Platform, nil
	}
	s.mockService.PlatformImage.OnListImagesOrDefault = func(name string) ([]string, error) {
		c.Assert(name, check.Equals, "myplatform")
		return []string{"tsuru/myplatform:v1", "tsuru/myplatform:v2"}, nil
	}
	request, err := http.NewRequest("GET", "/platforms/myplatform", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got result
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *PlatformSuite) TestPlatformInfoDefaultImage(c *check.C) {
	type result struct {
		Platform appTypes.Platform
		Images   []string
	}
	expected := result{
		Platform: appTypes.Platform{Name: "myPlatform"},
		Images:   []string{"tsuru/myplatform:latest"},
	}
	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		c.Assert(name, check.Equals, "myplatform")
		return &expected.Platform, nil
	}
	s.mockService.PlatformImage.OnListImagesOrDefault = func(name string) ([]string, error) {
		c.Assert(name, check.Equals, "myplatform")
		return []string{"tsuru/myplatform:latest"}, nil
	}
	request, err := http.NewRequest("GET", "/platforms/myplatform", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got result
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *PlatformSuite) TestPlatformRollback(c *check.C) {
	name := "myplatform"
	imageName := "tsuru/myplatform:v9"
	s.mockService.Platform.OnRollback = func(opts appTypes.PlatformOptions) error {
		c.Assert(opts.RollbackVersion, check.Equals, 9)
		c.Assert(opts.Name, check.Equals, name)
		return nil
	}
	v := url.Values{}
	v.Set("image", imageName)
	request, _ := http.NewRequest("POST", "/platforms/"+name+"/rollback", strings.NewReader(v.Encode()))
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(errors.New(msg.Error), check.ErrorMatches, "")
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePlatform, Value: name},
		Owner:  token.GetUserName(),
		Kind:   "platform.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": name},
		},
	}, eventtest.HasEvent)
}

func (s *PlatformSuite) TestPlatformRollbackNoImage(c *check.C) {
	name := "myplatform"
	s.mockService.Platform.OnRollback = func(opts appTypes.PlatformOptions) error {
		c.Errorf("service not expected to be called.")
		return nil
	}
	var buf bytes.Buffer
	request, err := http.NewRequest("POST", "/platforms/"+name+"/rollback", &buf)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *PlatformSuite) TestPlatformRollbackError(c *check.C) {
	name := "myplatform"
	s.mockService.Platform.OnRollback = func(opts appTypes.PlatformOptions) error {
		c.Errorf("service not expected to be called.")
		return nil
	}
	request, err := http.NewRequest("POST", "/platforms/"+name+"/rollback", nil)
	c.Assert(err, check.IsNil)
	token := createToken(c)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Matches, `(?s).*cannot rollback without an image name.*`)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}
