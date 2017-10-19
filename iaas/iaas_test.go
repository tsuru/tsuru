// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
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
	c.Assert(err, check.ErrorMatches, "IaaS provider \"abc\" based on \"abc\" not registered")
	c.Assert(provider, check.IsNil)
	RegisterIaasProvider("abc", newTestIaaS)
	provider, err = getIaasProvider("abc")
	c.Assert(err, check.IsNil)
	c.Assert(provider, check.FitsTypeOf, &TestIaaS{})
}

func (s *S) TestGetIaasProviderSameInstance(c *check.C) {
	RegisterIaasProvider("abc", newTestIaaS)
	provider, err := getIaasProvider("abc")
	c.Assert(err, check.IsNil)
	c.Assert(provider, check.FitsTypeOf, &TestIaaS{})
	provider2, err := getIaasProvider("abc")
	c.Assert(err, check.IsNil)
	value1 := reflect.ValueOf(provider)
	value2 := reflect.ValueOf(provider2)
	c.Assert(value1, check.Equals, value2)
}

func (s *S) TestDescribeNoDescriptiption(c *check.C) {
	RegisterIaasProvider("nodesc-iaas", newTestIaaS)
	desc, err := Describe("nodesc-iaas")
	c.Assert(err, check.IsNil)
	c.Assert(desc, check.Equals, "")
}

func (s *S) TestDescribe(c *check.C) {
	RegisterIaasProvider("withdesc-iaas", newTestDescriberIaaS)
	desc, err := Describe("withdesc-iaas")
	c.Assert(err, check.IsNil)
	c.Assert(desc, check.Equals, "ahoy desc!")
}

func (s *S) TestCustomizableIaaSProvider(c *check.C) {
	RegisterIaasProvider("customable-iaas", newTestCustomizableIaaS)
	config.Set("iaas:custom:abc:provider", "customable-iaas")
	defer config.Unset("iaas:custom:abc:provider")
	providerRoot, err := getIaasProvider("customable-iaas")
	c.Assert(err, check.IsNil)
	provider, err := getIaasProvider("abc")
	c.Assert(err, check.IsNil)
	c.Assert(provider, check.FitsTypeOf, TestCustomizableIaaS{})
	provider2, err := getIaasProvider("abc")
	c.Assert(err, check.IsNil)
	value1 := reflect.ValueOf(provider2)
	value2 := reflect.ValueOf(provider)
	value3 := reflect.ValueOf(providerRoot)
	c.Assert(value1, check.Equals, value2)
	c.Assert(value1, check.Not(check.Equals), value3)
}

func (s *S) TestReadUserDataDefault(c *check.C) {
	iaasInst := UserDataIaaS{}
	userData, err := iaasInst.ReadUserData(nil)
	c.Assert(err, check.IsNil)
	c.Assert(userData, check.Equals, defaultUserData)
}

func (s *S) TestReadUserData(c *check.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc def ghi")
	}))
	defer server.Close()
	config.Set("iaas:x:user-data", server.URL)
	defer config.Unset("iaas:x:user-data")
	userData, err := iaasInst.ReadUserData(nil)
	c.Assert(err, check.IsNil)
	c.Assert(userData, check.Equals, "abc def ghi")
}

func (s *S) TestReadUserDataEmpty(c *check.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	config.Set("iaas:x:user-data", "")
	defer config.Unset("iaas:x:user-data")
	userData, err := iaasInst.ReadUserData(nil)
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
	_, err := iaasInst.ReadUserData(nil)
	c.Assert(err, check.NotNil)
}

func (s *S) TestReadUserDataParamsOverride(c *check.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc def ghi")
	}))
	defer server.Close()
	config.Set("iaas:x:user-data", server.URL)
	defer config.Unset("iaas:x:user-data")
	params := map[string]string{"user-data": "myvalue"}
	userData, err := iaasInst.ReadUserData(params)
	c.Assert(err, check.IsNil)
	c.Assert(userData, check.Equals, "myvalue")
}

func (s *S) TestReadUserDataParamsURL(c *check.C) {
	iaasInst := UserDataIaaS{NamedIaaS: NamedIaaS{BaseIaaSName: "x"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "myurlvalue")
	}))
	defer server.Close()
	params := map[string]string{"user-data-url": server.URL}
	userData, err := iaasInst.ReadUserData(params)
	c.Assert(err, check.IsNil)
	c.Assert(userData, check.Equals, "myurlvalue")
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

func (s *S) TestGetConfig(c *check.C) {
	iaasInst := NamedIaaS{BaseIaaSName: "base-iaas"}
	config.Set("iaas:base-iaas:options", map[string]interface{}{
		"option1": "value1",
		"option2": 2,
	})
	val, err := iaasInst.GetConfig("options")
	c.Assert(err, check.IsNil)
	mapVal, ok := val.(map[string]interface{})
	c.Assert(ok, check.Equals, true)
	c.Assert(mapVal["option1"], check.Equals, "value1")
	c.Assert(mapVal["option2"], check.Equals, 2)
}

func (s *S) TestGetDefaultIaaSDefaultConfig(c *check.C) {
	config.Set("iaas:default", "default-iaas")
	val, err := getDefaultIaasName()
	c.Assert(err, check.IsNil)
	c.Assert(val, check.Equals, "default-iaas")
}

func (s *S) TestGetDefaultIaaSFailsWhenNoDefaultConfig(c *check.C) {
	val, err := getDefaultIaasName()
	c.Assert(err, check.DeepEquals, ErrNoDefaultIaaS)
	c.Assert(val, check.Equals, "")
}

func (s *S) TestGetDefaultIaaSSingleConfigured(c *check.C) {
	RegisterIaasProvider("customable-iaas-1", newTestCustomizableIaaS)
	RegisterIaasProvider("customable-iaas-2", newTestCustomizableIaaS)
	config.Set("iaas:customable-iaas-1", map[interface{}]interface{}{
		"conf": "abc",
	})
	defer config.Unset("iaas")
	val, err := getDefaultIaasName()
	c.Assert(err, check.IsNil)
	c.Assert(val, check.Equals, "customable-iaas-1")
}

func (s *S) TestGetDefaultIaaSSingleCustomConfigured(c *check.C) {
	RegisterIaasProvider("iaas-1", newTestCustomizableIaaS)
	config.Set("iaas:custom:custom-1", map[interface{}]interface{}{
		"provider": "iaas-1",
	})
	val, err := getDefaultIaasName()
	c.Assert(err, check.IsNil)
	c.Assert(val, check.Equals, "custom-1")
}

func (s *S) TestGetDefaultIaaSFailsMultipleConfigured(c *check.C) {
	RegisterIaasProvider("customable-iaas-1", newTestCustomizableIaaS)
	RegisterIaasProvider("customable-iaas-2", newTestCustomizableIaaS)
	config.Set("iaas:customable-iaas-1", map[interface{}]interface{}{
		"conf": "abc",
	})
	config.Set("iaas:customable-iaas-2", map[interface{}]interface{}{
		"conf": "def",
	})
	val, err := getDefaultIaasName()
	c.Assert(err, check.Equals, ErrNoDefaultIaaS)
	c.Assert(val, check.Equals, "")
}

func (s *S) TestGetDefaultIaaSFailsMultipleCustomConfigured(c *check.C) {
	RegisterIaasProvider("iaas-1", newTestCustomizableIaaS)
	config.Set("iaas:custom:custom-1", map[interface{}]interface{}{
		"provider": "iaas-1",
	})
	config.Set("iaas:custom:custom-2", map[interface{}]interface{}{
		"provider": "iaas-1",
	})
	val, err := getDefaultIaasName()
	c.Assert(err, check.Equals, ErrNoDefaultIaaS)
	c.Assert(val, check.Equals, "")
}

func (s *S) TestGetDefaultIaaSEc2MultipleConfigured(c *check.C) {
	RegisterIaasProvider("ec2", newTestCustomizableIaaS)
	RegisterIaasProvider("customable-iaas-1", newTestCustomizableIaaS)
	config.Set("iaas:ec2", map[interface{}]interface{}{
		"conf": "abc",
	})
	config.Set("iaas:customable-iaas-1", map[interface{}]interface{}{
		"conf": "def",
	})
	val, err := getDefaultIaasName()
	c.Assert(err, check.IsNil)
	c.Assert(val, check.Equals, "ec2")
}

func (s *S) TestStressConcurrentGet(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(1000)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	RegisterIaasProvider("customable-iaas", newTestCustomizableIaaS)
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
