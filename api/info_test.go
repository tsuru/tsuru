// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	_ "github.com/tsuru/tsuru/router/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestInfo(c *gocheck.C) {
	config.Set("autoscale", true)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/info", nil)
	c.Assert(err, gocheck.IsNil)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	fmt.Println(recorder)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/json")
	expected := map[string]interface{}{
		"autoscale": true,
	}
	var info map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &info)
	c.Assert(err, gocheck.IsNil)
	c.Assert(info, gocheck.DeepEquals, expected)
}
