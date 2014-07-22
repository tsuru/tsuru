// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cloudstack

import (
	"encoding/base64"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type cloudstackSuite struct{}

var _ = gocheck.Suite(&cloudstackSuite{})

func (s *cloudstackSuite) SetUpSuite(c *gocheck.C) {
	config.Set("iaas:cloudstack:api-key", "test")
	config.Set("iaas:cloudstack:secret-key", "test")
	config.Set("iaas:cloudstack:url", "test")
}

func (s *cloudstackSuite) TestReadUserDataDefault(c *gocheck.C) {
	userData, err := readUserData()
	c.Assert(err, gocheck.IsNil)
	c.Assert(userData, gocheck.Equals, base64.StdEncoding.EncodeToString([]byte(iaas.UserData)))
}

func (s *cloudstackSuite) TestReadUserData(c *gocheck.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc def ghi")
	}))
	defer server.Close()
	config.Set("iaas:cloudstack:user-data", server.URL)
	defer config.Unset("iaas:cloudstack:user-data")
	userData, err := readUserData()
	c.Assert(err, gocheck.IsNil)
	c.Assert(userData, gocheck.Equals, base64.StdEncoding.EncodeToString([]byte("abc def ghi")))
}

func (s *cloudstackSuite) TestReadUserDataError(c *gocheck.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	config.Set("iaas:cloudstack:user-data", server.URL)
	defer config.Unset("iaas:cloudstack:user-data")
	_, err := readUserData()
	c.Assert(err, gocheck.NotNil)
}

func (s *cloudstackSuite) TestCreateMachine(c *gocheck.C) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-type", "application/json")
		if strings.Contains(r.URL.RawQuery, "deployVirtualMachine") {
			fmt.Fprintln(w, `{"deployvirtualmachineresponse": {"id": "test", "jobid": "test"}}`)
		}
		if strings.Contains(r.URL.RawQuery, "listVirtualMachines") {
			json := `{ "listvirtualmachinesresponse" : { "count":1 ,"virtualmachine" : [  {"id":"0366ae09-0a77-4e2b-8595-3b749764a107","name":"vm-0366ae09-0a77-4e2b-8595-3b749764a107","projectid":"a98738c9-5acd-43e3-b1a1-972a3db5b196","project":"tsuru playground","domainid":"eec2dacf-9982-11e3-a2b8-eee0bc1594e0","domain":"ROOT","created":"2014-07-18T18:29:30-0300","state":"Stopped","haenable":false,"zoneid":"95046c6c-65b8-415f-99cb-0cff40dc5f9c","zonename":"RJOEBT0200BE","templateid":"99f66d4c-f923-46e5-aa7b-09a0b22ee747","templatename":"ubuntu-14.04-server-amd64","templatedisplaytext":"ubuntu 14.04 ( 3.13.0-24-generic )","passwordenabled":false,"serviceofferingid":"3ff651c8-a27f-4008-87d5-71636aaabbc6","serviceofferingname":"Medium","cpunumber":2,"cpuspeed":1800,"memory":8192,"guestosid":"eede1fdf-9982-11e3-a2b8-eee0bc1594e0","rootdeviceid":0,"rootdevicetype":"ROOT","securitygroup":[],"nic":[{"id":"40cd6225-9475-44a3-8288-d7a9a485d8ac","networkid":"18c20437-df18-4757-8435-1230248f955b","networkname":"PLAYGROUND_200BE","netmask":"255.255.255.0","gateway":"10.24.16.1","ipaddress":"10.24.16.241","isolationuri":"vlan://19","broadcasturi":"vlan://19","traffictype":"Guest","type":"Shared","isdefault":true,"macaddress":"06:54:7e:00:46:c6"}],"hypervisor":"XenServer","tags":[],"affinitygroup":[],"displayvm":true,"isdynamicallyscalable":true,"jobid":"82a574cc-43f2-440d-8774-e638065c37af","jobstatus":0} ] } }`
			fmt.Fprintln(w, json)
		}
	}))
	defer fakeServer.Close()
	config.Set("iaas:cloudstack:url", fakeServer.URL)
	var cs CloudstackIaaS
	params := map[string]string{"name": "test"}
	vm, err := cs.CreateMachine(params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(vm, gocheck.NotNil)
	c.Assert(vm.Address, gocheck.Equals, "10.24.16.241")
}

func (s *cloudstackSuite) TestBuildUrlToCloudstack(c *gocheck.C) {
	params := map[string]string{"atest": "2"}
	urlBuilded, err := buildUrl("commandTest", params)
	c.Assert(err, gocheck.IsNil)
	u, err := url.Parse(urlBuilded)
	c.Assert(err, gocheck.IsNil)
	q, err := url.ParseQuery(u.RawQuery)
	c.Assert(err, gocheck.IsNil)
	c.Assert(q["signature"], gocheck.NotNil)
	c.Assert(q["apiKey"], gocheck.NotNil)
	c.Assert(q["atest"], gocheck.NotNil)
	c.Assert(q["response"], gocheck.DeepEquals, []string{"json"})
	c.Assert(q["command"], gocheck.DeepEquals, []string{"commandTest"})
}
