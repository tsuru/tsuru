// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/quota"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

type QuotaSuite struct{}

var _ = gocheck.Suite(&QuotaSuite{})

func (QuotaSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_quota_api_tests")
}

func (QuotaSuite) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (QuotaSuite) TestQuotaByOwner(c *gocheck.C) {
	err := quota.Create("tank@elp.com", 3)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete("tank@elp.com")
	err = quota.Reserve("tank@elp.com", "tank/1")
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/quota/tank@elp.com?:owner=tank@elp.com", nil)
	c.Assert(err, gocheck.IsNil)
	err = quotaByOwner(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	result := map[string]interface{}{}
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result["available"], gocheck.Equals, float64(2))
	c.Assert(result["items"], gocheck.DeepEquals, []interface{}{"tank/1"})
}

func (QuotaSuite) TestQuotaNotFound(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/quota/raul@seixas.com?:owner=raul@seixas.com", nil)
	c.Assert(err, gocheck.IsNil)
	err = quotaByOwner(recorder, request, nil)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Quota not found$")
}

func (QuotaSuite) TestChangeQuota(c *gocheck.C) {
	err := quota.Create("tank@elp.com", 3)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete("tank@elp.com")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/quota/tank@elp.com?:owner=tank@elp.com", strings.NewReader("quota=1"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	err = changeQuota(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	_, qtd, err := quota.Items("tank@elp.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(int(qtd), gocheck.Equals, 1)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(body), gocheck.Equals, "Quota changed sucessfully.")
}

func (QuotaSuite) TestChangeQuotaWhenUserNotFound(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/quota/tank@elp.com?:owner=tank@elp.com", strings.NewReader("quota=1"))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	err = changeQuota(recorder, request, nil)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Quota not found$")
}

//strconv.ParseInt: parsing "": invalid syntax

func (QuotaSuite) TestChangeQuotaWhenAtoiParameterNotFound(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/quota/tank@elp.com?:owner=tank@elp.com", strings.NewReader("====="))
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	err = changeQuota(recorder, request, nil)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusInternalServerError)
	c.Assert(e, gocheck.ErrorMatches, "^invalid parameter$")
}
