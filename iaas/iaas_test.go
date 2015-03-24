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
	"runtime"
	"sync"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestRegisterIaasProvider(c *check.C) {
	provider, err := getIaasProvider("abc")
	c.Assert(err, check.ErrorMatches, "IaaS provider \"abc\" not registered")
	c.Assert(provider, check.IsNil)
	providerInstance := TestIaaS{}
	RegisterIaasProvider("abc", providerInstance)
	provider, err = getIaasProvider("abc")
	c.Assert(err, check.IsNil)
	c.Assert(provider, check.Equals, providerInstance)
}

func (s *S) TestDescribeNoDescriptiption(c *check.C) {
	providerInstance := TestIaaS{}
	RegisterIaasProvider("nodesc-iaas", providerInstance)
	desc, err := Describe("nodesc-iaas")
	c.Assert(err, check.IsNil)
	c.Assert(desc, check.Equals, "")
}

func (s *S) TestDescribe(c *check.C) {
	providerInstance := TestDescriberIaaS{}
	RegisterIaasProvider("withdesc-iaas", providerInstance)
	desc, err := Describe("withdesc-iaas")
	c.Assert(err, check.IsNil)
	c.Assert(desc, check.Equals, "ahoy desc!")
}

func (s *S) TestCustomizableIaaSProvider(c *check.C) {
	providerInstance := TestCustomizableIaaS{}
	RegisterIaasProvider("customable-iaas", providerInstance)
	config.Set("iaas:custom:abc:provider", "customable-iaas")
	defer config.Unset("iaas:custom:abc:provider")
	provider, err := getIaasProvider("abc")
	c.Assert(err, check.IsNil)
	c.Assert(provider, check.Not(check.DeepEquals), providerInstance)
	c.Assert(provider, check.FitsTypeOf, providerInstance)
	provider2, err := getIaasProvider("abc")
	c.Assert(err, check.IsNil)
	value1 := reflect.ValueOf(provider2)
	value2 := reflect.ValueOf(provider)
	c.Assert(value1, check.Equals, value2)
}

func (s *S) TestReadUserDataDefault(c *check.C) {
	iaasInst := UserDataIaaS{}
	userData, err := iaasInst.ReadUserData()
	c.Assert(err, check.IsNil)
	c.Assert(userData, check.Equals, base64.StdEncoding.EncodeToString([]byte(defaultUserData)))
}

func (s *S) TestReadUserData(c *check.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc def ghi")
	}))
	defer server.Close()
	config.Set("iaas:x:user-data", server.URL)
	defer config.Unset("iaas:x:user-data")
	userData, err := iaasInst.ReadUserData()
	c.Assert(err, check.IsNil)
	c.Assert(userData, check.Equals, base64.StdEncoding.EncodeToString([]byte("abc def ghi")))
}

func (s *S) TestReadUserDataEmpty(c *check.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	config.Set("iaas:x:user-data", "")
	defer config.Unset("iaas:x:user-data")
	userData, err := iaasInst.ReadUserData()
	c.Assert(err, check.IsNil)
	c.Assert(userData, check.Equals, "")
}

func (s *S) TestReadUserDataError(c *check.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	config.Set("iaas:x:user-data", server.URL)
	defer config.Unset("iaas:x:user-data")
	_, err := iaasInst.ReadUserData()
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetConfigString(c *check.C) {
	iaasInst := NamedIaaS{BaseIaaSName: "base-iaas"}
	config.Set("iaas:base-iaas:url", "default_url")
	val, err := iaasInst.GetConfigString("url")
	c.Assert(err, check.IsNil)
	c.Assert(val, check.Equals, "default_url")
	iaasInst.IaaSName = "something"
	val, err = iaasInst.GetConfigString("url")
	c.Assert(err, check.IsNil)
	c.Assert(val, check.Equals, "default_url")
	config.Set("iaas:custom:something:url", "custom_url")
	val, err = iaasInst.GetConfigString("url")
	c.Assert(err, check.IsNil)
	c.Assert(val, check.Equals, "custom_url")
}

func (s *S) TestStressConcurrentGet(c *check.C) {
	c.ExpectFailure("Concurrency bug under stress")
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1000))
	providerInstance := TestCustomizableIaaS{}
	RegisterIaasProvider("customable-iaas", providerInstance)
	config.Set("iaas:custom:abc:provider", "customable-iaas")
	defer config.Unset("iaas:custom:abc:provider")
	wg := sync.WaitGroup{}
	var firstVal IaaS
	var m sync.Mutex
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			provider, _ := getIaasProvider("abc")
			m.Lock()
			defer m.Unlock()
			if firstVal == nil {
				firstVal = provider
			} else {
				c.Assert(reflect.ValueOf(firstVal), check.Equals, reflect.ValueOf(provider))
			}
		}()
	}
	wg.Wait()
}
