// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"

	check "gopkg.in/check.v1"
)

func (s *S) TestDumpGoroutines(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/debug/goroutines", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, `(?s)goroutine \d+ \[running\]:.*`)
}
