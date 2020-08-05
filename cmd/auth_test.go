// Copyright 2012 tsuru authors. All rights reserved.
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
	"github.com/tsuru/tsuru/fs"
	"github.com/tsuru/tsuru/fs/fstest"
	check "gopkg.in/check.v1"
)

func nativeScheme() {
	os.Setenv("TSURU_AUTH_SCHEME", "")
}

func TargetInit(fsystem fs.Fs) {
	f, _ := fsystem.Create(JoinWithUserDir(".tsuru", "target"))
	f.Write([]byte("http://localhost"))
	f.Close()
	WriteOnTargetList("test", "http://localhost")
}

func (s *S) TestNativeLogin(c *check.C) {
	os.Unsetenv("TSURU_TOKEN")
	nativeScheme()
	fsystem = &fstest.RecordingFs{FileContent: "old-token"}
	TargetInit(fsystem)
	defer func() {
		fsystem = nil
	}()
	expected := "Password: \nSuccessfully logged in!\n"
	reader := strings.NewReader("chico\n")
	context := Context{[]string{"foo@foo.com"}, globalManager.stdout, globalManager.stderr, reader}
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{
			Message: `{"token": "sometoken", "is_admin": true}`,
			Status:  http.StatusOK,
		},
		CondFunc: func(r *http.Request) bool {
			contentType := r.Header.Get("Content-Type") == "application/x-www-form-urlencoded"
			password := r.FormValue("password") == "chico"
			url := r.URL.Path == "/1.0/users/foo@foo.com/tokens"
			return contentType && password && url
		},
	}
	client := NewClient(&http.Client{Transport: &transport}, nil, globalManager)
	command := login{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	token, err := ReadToken()
	c.Assert(err, check.IsNil)
	c.Assert(token, check.Equals, "sometoken")
}

func (s *S) TestNativeLoginWithoutEmailFromArg(c *check.C) {
	os.Unsetenv("TSURU_TOKEN")
	nativeScheme()
	fsystem = &fstest.RecordingFs{}
	TargetInit(fsystem)
	defer func() {
		fsystem = nil
	}()
	expected := "Email: Password: \nSuccessfully logged in!\n"
	reader := strings.NewReader("chico@tsuru.io\nchico\n")
	context := Context{[]string{}, globalManager.stdout, globalManager.stderr, reader}
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{
			Message: `{"token": "sometoken", "is_admin": true}`,
			Status:  http.StatusOK,
		},
		CondFunc: func(r *http.Request) bool {
			return r.URL.Path == "/1.0/users/chico@tsuru.io/tokens"
		},
	}
	client := NewClient(&http.Client{Transport: &transport}, nil, globalManager)
	command := login{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	token, err := ReadToken()
	c.Assert(err, check.IsNil)
	c.Assert(token, check.Equals, "sometoken")
}

func (s *S) TestNativeLoginShouldNotDependOnTsuruTokenFile(c *check.C) {
	nativeScheme()
	rfs := &fstest.RecordingFs{}
	f, _ := rfs.Create(JoinWithUserDir(".tsuru", "target"))
	f.Write([]byte("http://localhost"))
	f.Close()
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := "Password: \nSuccessfully logged in!\n"
	reader := strings.NewReader("chico\n")
	context := Context{[]string{"foo@foo.com"}, globalManager.stdout, globalManager.stderr, reader}
	client := NewClient(&http.Client{Transport: &cmdtest.Transport{Message: `{"token":"anothertoken"}`, Status: http.StatusOK}}, nil, globalManager)
	command := login{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestNativeLoginShouldReturnErrorIfThePasswordIsNotGiven(c *check.C) {
	nativeScheme()
	context := Context{[]string{"foo@foo.com"}, globalManager.stdout, globalManager.stderr, strings.NewReader("\n")}
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
	os.Setenv("TSURU_TARGET", "localhost:8080")
	expected := "Successfully logged out!\n"
	context := Context{[]string{}, globalManager.stdout, globalManager.stderr, globalManager.stdin}
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
	client := NewClient(&http.Client{Transport: &transport}, nil, globalManager)
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(rfs.HasAction("remove "+JoinWithUserDir(".tsuru", "token")), check.Equals, true)
	c.Assert(called, check.Equals, true)
}

func (s *S) TestLogoutWhenNotLoggedIn(c *check.C) {
	os.Unsetenv("TSURU_TOKEN")
	os.Unsetenv("TSURU_TARGET")
	fsystem = &fstest.FileNotFoundFs{}
	defer func() {
		fsystem = nil
	}()
	context := Context{[]string{}, globalManager.stdout, globalManager.stderr, globalManager.stdin}
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
	context := Context{[]string{}, globalManager.stdout, globalManager.stderr, globalManager.stdin}
	command := logout{}
	transport := cmdtest.Transport{Message: "", Status: http.StatusOK}
	client := NewClient(&http.Client{Transport: &transport}, nil, globalManager)
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(rfs.HasAction("remove "+JoinWithUserDir(".tsuru", "token")), check.Equals, true)
}

func (s *S) TestLoginGetSchemeCachesResult(c *check.C) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"name": "oauth", "data": {}}`))
	}))
	defer ts.Close()
	os.Setenv("TSURU_TARGET", ts.URL)
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
	os.Setenv("TSURU_TARGET", ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "oauth")
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "native", "data": {}}`))
	}))
	defer ts.Close()
	os.Setenv("TSURU_TARGET", ts.URL)
	loginCmd = login{}
	scheme = loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "native")
}

func (s *S) TestLoginGetSchemeParsesData(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": "y", "z": "w"}}`))
	}))
	defer ts.Close()
	os.Setenv("TSURU_TARGET", ts.URL)
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
	os.Setenv("TSURU_TARGET", ts.URL)
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
	os.Setenv("TSURU_TARGET", ts.URL)
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
	os.Setenv("TSURU_TARGET", ts.URL)
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
