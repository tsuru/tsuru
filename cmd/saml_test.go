// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"

	check "gopkg.in/check.v1"
)

func (s *S) TestSamlScheme(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "saml", "data" : {
												"request_id": "0123456789",
												"request_timeout": "3",
												"saml_request": "SamlData"
										  }
						}`))
	}))
	defer ts.Close()
	os.Setenv("TSURU_TARGET", ts.URL)
	loginCmd := login{}
	scheme := loginCmd.getScheme()
	c.Assert(scheme.Name, check.Equals, "saml")
	c.Assert(scheme.Data["request_id"], check.Equals, "0123456789")
	c.Assert(scheme.Data["request_timeout"], check.Equals, "3")
	c.Assert(scheme.Data["saml_request"], check.Equals, "SamlData")
}
