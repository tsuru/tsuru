// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package openstack

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type openstackSuite struct{}

var _ = gocheck.Suite(&openstackSuite{})

func (s *openstackSuite) SetUpSuite(c *gocheck.C) {
	config.Set("iaas:openstack:api_endpoint", "test")
	config.Set("iaas:openstack:Username", "test")
	config.Set("iaas:openstack:ProjectName", "test")
	config.Set("iaas:openstack:Password", "test")
}

func (s *openstackSuite) TestCreateMachine(c *gocheck.C) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		RequestURI := r.RequestURI

		w.Header().Set("Content-type", "application/json")
		if method == "POST" {

			if strings.Contains(RequestURI, "servers") {
				json := `{ "server": {
						"security_groups": [ { "name": "default" } ], "OS-DCF:diskConfig": "MANUAL", "id": "c6d04159-9bfc-4ab8-823d-0d5ca2abe152", "links": [ { "href": "http://166.78.46.130:8774/v2/4fd44f30292945e481c7b8a0c8908869/servers/c6d04159-9bfc-4ab8-823d-0d5ca2abe152", "rel": "self" }, { "href": "http://166.78.46.130:8774/4fd44f30292945e481c7b8a0c8908869/servers/c6d04159-9bfc-4ab8-823d-0d5ca2abe152", "rel": "bookmark" } ], "adminPass": "aabbccddeeff" } } `
				fmt.Fprintln(w, json)

			} else {
				json := `{ "access": { "token": { "issued_at": "2014-01-30T15:30:58.819584", "expires": "2014-01-31T15:30:58Z", "id": "aaaaa-bbbbb-ccccc-dddd", "tenant": { "description": null, "enabled": true, "id": "fc394f2ab2df4114bde39905f800dc57", "name": "demo" } }, "serviceCatalog": [ { "Endpoints": [ { "publicURL": "http://` + r.Host + `/v2/fc394f2ab2df4114bde39905f800dc57" } ], "endpoints_links": [], "type": "compute", "name": "nova" } ] } } `
				fmt.Fprintln(w, json)
			}
		} else if method == "GET" {
			if strings.Contains(RequestURI, "c6d04159-9bfc-4ab8-823d-0d5ca2abe152") { //vm ID
				json := `{"server":{ "addresses":{"test":  [{ "addr":"10.1.1.1" }] }, "name":"test",           "status":"RUNNING", "OS-EXT-AZ:availability_zone":"test"}}`
				fmt.Fprintln(w, json)
			}
		}
	}))
	defer fakeServer.Close()
	config.Set("iaas:openstack:url", fakeServer.URL)

	config.Set("iaas:openstack:api_endpoint", fakeServer.URL)
	var cs OpenstackIaaS
	params := map[string]string{
		"projectid":  "val",
		"networkids": "val",
		"templateid": "val",
		"zoneid":     "val",
	}
	vm, err := cs.CreateMachine(params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(vm, gocheck.NotNil)
	c.Assert(vm.Address, gocheck.Equals, "10.1.1.1")
	c.Assert(vm.Id, gocheck.Equals, "test")
	fakeServer.Close()
}

//func (s *openstackSuite) TestCreateMachineValidateParams(c *gocheck.C) {
//	var cs OpenstackIaaS
//	params := map[string]string{
//		"name": "something",
//	}
//	_, err := cs.CreateMachine(params)
//	c.Assert(err, gocheck.ErrorMatches, "param \"projectid\" is mandatory")
//}

func (s *openstackSuite) TestDeleteMachine(c *gocheck.C) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		RequestURI := r.RequestURI

		w.Header().Set("Content-type", "application/json")
		if method == "POST" {

			if strings.Contains(RequestURI, "servers") {
				json := `{ "server": {
						"security_groups": [ { "name": "default" } ], "OS-DCF:diskConfig": "MANUAL", "id": "c6d04159-9bfc-4ab8-823d-0d5ca2abe152", "links": [ { "href": "http://166.78.46.130:8774/v2/4fd44f30292945e481c7b8a0c8908869/servers/c6d04159-9bfc-4ab8-823d-0d5ca2abe152", "rel": "self" }, { "href": "http://166.78.46.130:8774/4fd44f30292945e481c7b8a0c8908869/servers/c6d04159-9bfc-4ab8-823d-0d5ca2abe152", "rel": "bookmark" } ], "adminPass": "aabbccddeeff" } } `
				fmt.Fprintln(w, json)

			} else {
				json := `{ "access": { "token": { "issued_at": "2014-01-30T15:30:58.819584", "expires": "2014-01-31T15:30:58Z", "id": "aaaaa-bbbbb-ccccc-dddd", "tenant": { "description": null, "enabled": true, "id": "fc394f2ab2df4114bde39905f800dc57", "name": "demo" } }, "serviceCatalog": [ { "Endpoints": [ { "publicURL": "http://` + r.Host + `/v2/fc394f2ab2df4114bde39905f800dc57" } ], "endpoints_links": [], "type": "compute", "name": "nova" } ] } } `
				fmt.Fprintln(w, json)
			}
		} else if method == "GET" {
			if strings.Contains(RequestURI, "c6d04159-9bfc-4ab8-823d-0d5ca2abe152") { //vm ID
				//json := `{"server":{ "addresses": {"test": "[ { "addr": "10.1.1.1" }] },"status":"RUNNING", "OS-EXT-AZ:availability_zone":"test"}}`
				json := `{"server":{ "addresses":{"test":  [{ "addr":"10.1.1.1" }] }, "name":"test","status":"RUNNING", "OS-EXT-AZ:availability_zone":"test"}}`

				fmt.Fprintln(w, json)
			} else if strings.Contains(RequestURI, "servers?") {
				json := `{"servers":[{"id":"c6d04159-9bfc-4ab8-823d-0d5ca2abe152"}]}`
				fmt.Fprintln(w, json)
			}
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer fakeServer.Close()
	config.Set("iaas:openstack:url", fakeServer.URL)

	config.Set("iaas:openstack:api_endpoint", fakeServer.URL)
	var cs OpenstackIaaS
	machine := iaas.Machine{Id: "test", CreationParams: map[string]string{"projectid": "test"}}
	err := cs.DeleteMachine(&machine)
	c.Assert(err, gocheck.IsNil)
}
