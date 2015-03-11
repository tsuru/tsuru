// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

type IndexSuite struct{}

var _ = check.Suite(IndexSuite{})

func (IndexSuite) SetUpTest(c *check.C) {
	config.Set("host", "http://localhost/")
	config.Set("auth:user-registration", true)
	config.Set("auth:scheme", "native")
	config.Set("repo-manager", "gandalf")
}

func (IndexSuite) TestIndex(c *check.C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var expected bytes.Buffer
	indexTemplate.Execute(&expected, map[string]interface{}{
		"tsuruTarget": "http://localhost/",
		"userCreate":  true,
		"nativeLogin": true,
		"keysEnabled": true,
	})
	c.Assert(recorder.Body.String(), check.Equals, expected.String())
}

func (IndexSuite) TestIndexNoRepoManager(c *check.C) {
	config.Unset("repo-manager")
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var expected bytes.Buffer
	indexTemplate.Execute(&expected, map[string]interface{}{
		"tsuruTarget": "http://localhost/",
		"userCreate":  true,
		"nativeLogin": true,
		"keysEnabled": true,
	})
	c.Assert(recorder.Body.String(), check.Equals, expected.String())
}

func (IndexSuite) TestIndexNoUserCreation(c *check.C) {
	config.Set("auth:user-registration", false)
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var expected bytes.Buffer
	indexTemplate.Execute(&expected, map[string]interface{}{
		"tsuruTarget": "http://localhost/",
		"userCreate":  false,
		"nativeLogin": true,
		"keysEnabled": true,
	})
	c.Assert(recorder.Body.String(), check.Equals, expected.String())
}

func (IndexSuite) TestIndexNoGandalf(c *check.C) {
	config.Set("repo-manager", "none")
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var expected bytes.Buffer
	indexTemplate.Execute(&expected, map[string]interface{}{
		"tsuruTarget": "http://localhost/",
		"userCreate":  true,
		"nativeLogin": true,
		"keysEnabled": false,
	})
	c.Assert(recorder.Body.String(), check.Equals, expected.String())
}

func (IndexSuite) TestIndexNoAuthScheme(c *check.C) {
	config.Unset("auth:scheme")
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var expected bytes.Buffer
	indexTemplate.Execute(&expected, map[string]interface{}{
		"tsuruTarget": "http://localhost/",
		"userCreate":  true,
		"nativeLogin": true,
		"keysEnabled": true,
	})
	c.Assert(recorder.Body.String(), check.Equals, expected.String())
}

func (IndexSuite) TestIndexOAuth(c *check.C) {
	config.Set("auth:scheme", "oauth")
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var expected bytes.Buffer
	indexTemplate.Execute(&expected, map[string]interface{}{
		"tsuruTarget": "http://localhost/",
		"userCreate":  true,
		"nativeLogin": false,
		"keysEnabled": true,
	})
	c.Assert(recorder.Body.String(), check.Equals, expected.String())
}

func (IndexSuite) TestIndexCustomTemplate(c *check.C) {
	config.Set("index-page-template", "testdata/index.html")
	defer config.Unset("index-page-template")
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusOK)
	index := template.Must(template.ParseFiles("testdata/index.html"))
	var expected bytes.Buffer
	index.Execute(&expected, map[string]interface{}{
		"tsuruTarget": "http://localhost/",
		"userCreate":  true,
		"nativeLogin": true,
		"keysEnabled": true,
	})
	c.Assert(recorder.Body.String(), check.Equals, expected.String())
}

func (IndexSuite) TestIndexTemplateError(c *check.C) {
	config.Set("index-page-template", "testdata/not-found.html")
	defer config.Unset("index-page-template")
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "open testdata/not-found.html: no such file or directory\n")
}

func (IndexSuite) TestIndexConfigFunction(c *check.C) {
	config.Set("test:name", "Gopher")
	config.Set("test:age", 10)
	config.Set("test:weight", 32.05)
	config.Set("test:married", true)
	expected := `Gopher
10
32.05
true
`
	config.Set("index-page-template", "testdata/index_config.html")
	defer config.Unset("index-page-template")
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, expected)
}

func (IndexSuite) TestIndexDisabled(c *check.C) {
	config.Set("disable-index-page", true)
	defer config.Unset("disable-index-page")
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}
