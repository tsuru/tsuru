// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/tsuru/tsuru/cmd/cmdtest"
	"github.com/tsuru/tsuru/fs/fstest"
	"launchpad.net/gocheck"
)

func navitveScheme() {
	os.Setenv("TSURU_AUTH_SCHEME", "")
}

func (s *S) TestLoginInfo(c *gocheck.C) {
	c.Assert((&login{}).Info().Usage, gocheck.Equals, "login <email>")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	c.Assert((&login{}).Info().Usage, gocheck.Equals, "login")
}

func (s *S) TestLoginName(c *gocheck.C) {
	c.Assert((&login{}).Name(), gocheck.Equals, "login")
}

func (s *S) TestNativeLogin(c *gocheck.C) {
	navitveScheme()
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
	token, err := ReadToken()
	c.Assert(err, gocheck.IsNil)
	c.Assert(token, gocheck.Equals, "sometoken")
}

func (s *S) TestNativeLoginShouldNotDependOnTsuruTokenFile(c *gocheck.C) {
	navitveScheme()
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
}

func (s *S) TestNativeLoginShouldReturnErrorIfThePasswordIsNotGiven(c *gocheck.C) {
	navitveScheme()
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, strings.NewReader("\n")}
	command := login{}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^You must provide the password!$")
}

func (s *S) TestLogout(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
	c.Assert(rfs.HasAction("remove "+JoinWithUserDir(".tsuru_token")), gocheck.Equals, true)
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestLogoutWhenNotLoggedIn(c *gocheck.C) {
	fsystem = &fstest.FileNotFoundFs{}
	defer func() {
		fsystem = nil
	}()
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := logout{}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "You're not logged in!")
}

func (s *S) TestLogoutNoTarget(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
	c.Assert(rfs.HasAction("remove "+JoinWithUserDir(".tsuru_token")), gocheck.Equals, true)
}

func (s *S) TestLoginGetSchemeCachesResult(c *gocheck.C) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"name": "oauth", "data": {}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, gocheck.Equals, "oauth")
	c.Assert(scheme.Data, gocheck.DeepEquals, map[string]string{})
	c.Assert(callCount, gocheck.Equals, 1)
	loginCmd.getScheme()
	c.Assert(callCount, gocheck.Equals, 1)
}

func (s *S) TestLoginGetSchemeDefault(c *gocheck.C) {
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, gocheck.Equals, "native")
	c.Assert(scheme.Data, gocheck.DeepEquals, map[string]string{})
}

func (s *S) TestLoginGetScheme(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, gocheck.Equals, "oauth")
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "native", "data": {}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd = login{}
	scheme = loginCmd.getScheme()
	c.Assert(scheme.Name, gocheck.Equals, "native")
}

func (s *S) TestLoginGetSchemeParsesData(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": "y", "z": "w"}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, gocheck.Equals, "oauth")
	c.Assert(scheme.Data, gocheck.DeepEquals, map[string]string{"x": "y", "z": "w"})
}

func (s *S) TestLoginGetSchemeInvalidData(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": 9, "z": "w"}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, gocheck.Equals, "native")
	c.Assert(scheme.Data, gocheck.DeepEquals, map[string]string{})
}

func (s *S) TestSchemeInfo(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": "y"}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	info, err := schemeInfo()
	c.Assert(err, gocheck.IsNil)
	c.Assert(info.Name, gocheck.Equals, "oauth")
	c.Assert(info.Data, gocheck.DeepEquals, map[string]string{"x": "y"})
}

func (s *S) TestSchemeInfoInvalidData(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"x": 9}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	_, err := schemeInfo()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestReadTokenEnvironmentVariable(c *gocheck.C) {
	os.Setenv("TSURU_TOKEN", "ABCDEFGH")
	defer os.Setenv("TSURU_TOKEN", "")
	token, err := ReadToken()
	c.Assert(err, gocheck.IsNil)
	c.Assert(token, gocheck.Equals, "ABCDEFGH")
}
