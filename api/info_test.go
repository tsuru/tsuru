// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	_ "github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func (s *S) TestInfo(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/info", nil)
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	expected := map[string]interface{}{
		"version":   Version,
	}
	var info map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &info)
	c.Assert(err, check.IsNil)
	c.Assert(info, check.DeepEquals, expected)
}
