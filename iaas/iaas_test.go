// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"

	"github.com/tsuru/config"
	"launchpad.net/gocheck"
)

func (s *S) TestRegisterIaasProvider(c *gocheck.C) {
	provider, err := GetIaasProvider("abc")
	c.Assert(err, gocheck.ErrorMatches, "IaaS provider \"abc\" not registered")
	c.Assert(provider, gocheck.IsNil)
	providerInstance := TestIaaS{}
	RegisterIaasProvider("abc", providerInstance)
	provider, err = GetIaasProvider("abc")
	c.Assert(err, gocheck.IsNil)
	c.Assert(provider, gocheck.Equals, providerInstance)
}

func (s *S) TestDescribeNoDescriptiption(c *gocheck.C) {
	providerInstance := TestIaaS{}
	RegisterIaasProvider("nodesc-iaas", providerInstance)
	desc, err := Describe("nodesc-iaas")
	c.Assert(err, gocheck.IsNil)
	c.Assert(desc, gocheck.Equals, "")
}

func (s *S) TestDescribe(c *gocheck.C) {
	providerInstance := TestDescriberIaaS{}
	RegisterIaasProvider("withdesc-iaas", providerInstance)
	desc, err := Describe("withdesc-iaas")
	c.Assert(err, gocheck.IsNil)
	c.Assert(desc, gocheck.Equals, "ahoy desc!")
}

func (s *S) TestCustomizableIaaSProvider(c *gocheck.C) {
	providerInstance := TestCustomizableIaaS{}
	RegisterIaasProvider("customable-iaas", providerInstance)
	config.Set("iaas:custom:abc:provider", "customable-iaas")
	defer config.Unset("iaas:custom:abc:provider")
	provider, err := GetIaasProvider("abc")
	c.Assert(err, gocheck.IsNil)
	c.Assert(provider, gocheck.Not(gocheck.DeepEquals), providerInstance)
	c.Assert(provider, gocheck.FitsTypeOf, providerInstance)
	provider2, err := GetIaasProvider("abc")
	c.Assert(err, gocheck.IsNil)
	value1 := reflect.ValueOf(provider2)
	value2 := reflect.ValueOf(provider)
	c.Assert(value1, gocheck.Equals, value2)
}

func (s *S) TestReadUserDataDefault(c *gocheck.C) {
	iaasInst := UserDataIaaS{}
	userData, err := iaasInst.ReadUserData()
	c.Assert(err, gocheck.IsNil)
	c.Assert(userData, gocheck.Equals, base64.StdEncoding.EncodeToString([]byte(defaultUserData)))
}

func (s *S) TestReadUserData(c *gocheck.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc def ghi")
	}))
	defer server.Close()
	config.Set("iaas:x:user-data", server.URL)
	defer config.Unset("iaas:x:user-data")
	userData, err := iaasInst.ReadUserData()
	c.Assert(err, gocheck.IsNil)
	c.Assert(userData, gocheck.Equals, base64.StdEncoding.EncodeToString([]byte("abc def ghi")))
}

func (s *S) TestReadUserDataError(c *gocheck.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	config.Set("iaas:x:user-data", server.URL)
	defer config.Unset("iaas:x:user-data")
	_, err := iaasInst.ReadUserData()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestGetConfigString(c *gocheck.C) {
	iaasInst := NamedIaaS{BaseIaaSName: "base-iaas"}
	config.Set("iaas:base-iaas:url", "default_url")
	val, err := iaasInst.GetConfigString("url")
	c.Assert(err, gocheck.IsNil)
	c.Assert(val, gocheck.Equals, "default_url")
	iaasInst.IaaSName = "something"
	val, err = iaasInst.GetConfigString("url")
	c.Assert(err, gocheck.IsNil)
	c.Assert(val, gocheck.Equals, "default_url")
	config.Set("iaas:custom:something:url", "custom_url")
	val, err = iaasInst.GetConfigString("url")
	c.Assert(err, gocheck.IsNil)
	c.Assert(val, gocheck.Equals, "custom_url")
}
