// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tsuru/tsuru/cmd/cmdtest"
	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
)

func nativeScheme() {
	os.Setenv("TSURU_AUTH_SCHEME", "")
}

func (s *S) TestNativeLogin(c *check.C) {
	nativeScheme()
	fsystem = &fstest.RecordingFs{FileContent: "old-token"}
	defer func() {
		fsystem = nil
	}()
	expected := "Password: \nSuccessfully logged in!\n"
	reader := strings.NewReader("chico\n")
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, reader}
	client := NewClient(&http.Client{Transport: &cmdtest.Transport{Message: `{"token": "sometoken", "is_admin": true}`, Status: http.StatusOK}}, nil, manager)
	command := login{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	token, err := ReadToken()
	c.Assert(err, check.IsNil)
	c.Assert(token, check.Equals, "sometoken")
}

func (s *S) TestNativeLoginWithoutEmailFromArg(c *check.C) {
	nativeScheme()
	fsystem = &fstest.RecordingFs{FileContent: "old-token"}
	defer func() {
		fsystem = nil
	}()
	expected := "Email: Password: \nSuccessfully logged in!\n"
	reader := strings.NewReader("chico@tsuru.io\nchico\n")
	context := Context{[]string{}, manager.stdout, manager.stderr, reader}
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{
			Message: `{"token": "sometoken", "is_admin": true}`,
			Status:  http.StatusOK,
		},
		CondFunc: func(r *http.Request) bool {
			return r.URL.Path == "/users/chico@tsuru.io/tokens"
		},
	}
	client := NewClient(&http.Client{Transport: &transport}, nil, manager)
	command := login{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	token, err := ReadToken()
	c.Assert(err, check.IsNil)
	c.Assert(token, check.Equals, "sometoken")
}

func (s *S) TestNativeLoginShouldNotDependOnTsuruTokenFile(c *check.C) {
	nativeScheme()
	rfs := &fstest.RecordingFs{}
	f, _ := rfs.Create(JoinWithUserDir(".tsuru_target"))
	f.Write([]byte("http://localhost"))
	f.Close()
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := "Password: \nSuccessfully logged in!\n"
	reader := strings.NewReader("chico\n")
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, reader}
	client := NewClient(&http.Client{Transport: &cmdtest.Transport{Message: `{"token":"anothertoken"}`, Status: http.StatusOK}}, nil, manager)
	command := login{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestNativeLoginShouldReturnErrorIfThePasswordIsNotGiven(c *check.C) {
	nativeScheme()
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, strings.NewReader("\n")}
	command := login{}
	err := command.Run(&context, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^You must provide the password!$")
}

func (s *S) TestLogout(c *check.C) {
	var called bool
	rfs := &fstest.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	writeToken("mytoken")
	writeTarget("localhost:8080")
	expected := "Successfully logged out!\n"
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := logout{}
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{
			Message: "",
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "DELETE" && req.URL.Path == "/users/tokens" &&
				req.Header.Get("Authorization") == "bearer mytoken"
		},
	}
	client := NewClient(&http.Client{Transport: &transport}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(rfs.HasAction("remove "+JoinWithUserDir(".tsuru_token")), check.Equals, true)
	c.Assert(called, check.Equals, true)
}

func (s *S) TestLogoutWhenNotLoggedIn(c *check.C) {
	fsystem = &fstest.FileNotFoundFs{}
	defer func() {
		fsystem = nil
	}()
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := logout{}
	err := command.Run(&context, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "You're not logged in!")
}

func (s *S) TestLogoutNoTarget(c *check.C) {
	rfs := &fstest.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	writeToken("mytoken")
	expected := "Successfully logged out!\n"
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := logout{}
	transport := cmdtest.Transport{Message: "", Status: http.StatusOK}
	client := NewClient(&http.Client{Transport: &transport}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(rfs.HasAction("remove "+JoinWithUserDir(".tsuru_token")), check.Equals, true)
}

func (s *S) TestLoginGetSchemeCachesResult(c *check.C) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"name": "oauth", "data": {}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "oauth")
	c.Assert(scheme.Data, check.DeepEquals, map[string]string{})
	c.Assert(callCount, check.Equals, 1)
	loginCmd.getScheme()
	c.Assert(callCount, check.Equals, 1)
}

func (s *S) TestLoginGetSchemeDefault(c *check.C) {
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "native")
	c.Assert(scheme.Data, check.DeepEquals, map[string]string{})
}

func (s *S) TestLoginGetScheme(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "oauth")
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "native", "data": {}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd = login{}
	scheme = loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "native")
}

func (s *S) TestLoginGetSchemeParsesData(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": "y", "z": "w"}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "oauth")
	c.Assert(scheme.Data, check.DeepEquals, map[string]string{"x": "y", "z": "w"})
}

func (s *S) TestLoginGetSchemeInvalidData(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": 9, "z": "w"}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "native")
	c.Assert(scheme.Data, check.DeepEquals, map[string]string{})
}

func (s *S) TestSchemeInfo(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": "y"}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	info, err := schemeInfo()
	c.Assert(err, check.IsNil)
	c.Assert(info.Name, check.Equals, "oauth")
	c.Assert(info.Data, check.DeepEquals, map[string]string{"x": "y"})
}

func (s *S) TestSchemeInfoInvalidData(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": 9}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	_, err := schemeInfo()
	c.Assert(err, check.NotNil)
}

func (s *S) TestReadTokenEnvironmentVariable(c *check.C) {
	os.Setenv("TSURU_TOKEN", "ABCDEFGH")
	defer os.Setenv("TSURU_TOKEN", "")
	token, err := ReadToken()
	c.Assert(err, check.IsNil)
	c.Assert(token, check.Equals, "ABCDEFGH")
}

func (s *S) TestUserInfoRun(c *check.C) {
	var called bool
	expected := `Email: myuser@company.com
Teams: frontend, backend, sysadmin, full stack
`
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := userInfo{}
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{
			Message: `{"Email":"myuser@company.com","Teams":["frontend","backend","sysadmin","full stack"]}`,
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "GET" && req.URL.Path == "/users/info"
		},
	}
	client := NewClient(&http.Client{Transport: &transport}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(called, check.Equals, true)
}

func (s *S) TestPasswordFromReaderUsingFile(c *check.C) {
	tmpdir, err := filepath.EvalSymlinks(os.TempDir())
	filename := path.Join(tmpdir, "password-reader.txt")
	c.Assert(err, check.IsNil)
	file, err := os.Create(filename)
	c.Assert(err, check.IsNil)
	defer os.Remove(filename)
	file.WriteString("hello")
	file.Seek(0, 0)
	password, err := PasswordFromReader(file)
	c.Assert(err, check.IsNil)
	c.Assert(password, check.Equals, "hello")
}

func (s *S) TestPasswordFromReaderUsingStringsReader(c *check.C) {
	reader := strings.NewReader("abcd\n")
	password, err := PasswordFromReader(reader)
	c.Assert(err, check.IsNil)
	c.Assert(password, check.Equals, "abcd")
}
