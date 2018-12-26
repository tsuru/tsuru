// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package digitalocean

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

func (s *digitaloceanSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("iaas:digitalocean:token", "test")
}

type digitaloceanSuite struct{}

var _ = check.Suite(&digitaloceanSuite{})

func (s *digitaloceanSuite) TestCreateMachine(c *check.C) {
	var createRequest map[string]interface{}
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/droplets" {
			err := json.NewDecoder(r.Body).Decode(&createRequest)
			c.Assert(err, check.IsNil)
			fmt.Fprintln(w, `{"droplet": {"id": 1, "status": "new", "networks": {"v4": [], "v6": []}}}`)
		}
		if r.URL.Path == "/v2/droplets/1" {
			fmt.Fprintln(w, `{"droplet": {"id": 1, "status": "active", "networks": {"v4": [{"ip_address": "104.131.186.241", "netmask": "255.255.240.0", "gateway": "104.131.176.1", "type": "public"}], "v6": [{"ip_address": "2604:A880:0800:0010:0000:0000:031D:2001", "netmask": 64, "gateway": "2604:A880:0800:0010:0000:0000:0000:0001", "type": "public"}]}}}`)
		}
	}))
	defer fakeServer.Close()
	config.Set("iaas:digitalocean:url", fakeServer.URL)
	do := newDigitalOceanIaas("digitalocean")
	params := map[string]string{
		"name":     "example.com",
		"region":   "nyc3",
		"size":     "512mb",
		"image":    "ubuntu-14-04-x64",
		"ssh-keys": "5050,2032,07:b9:a1:65:1b,13",
	}
	m, err := do.CreateMachine(params)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.NotNil)
	c.Assert(m.Address, check.Equals, "104.131.186.241")
	c.Assert(m.Id, check.Equals, "1")
	c.Assert(m.Status, check.Equals, "active")
	expectedKeys := []interface{}{float64(5050), float64(2032), "07:b9:a1:65:1b", float64(13)}
	c.Assert(createRequest["ssh_keys"], check.DeepEquals, expectedKeys)
	c.Assert(createRequest["private_networking"], check.Equals, false)
}

func (s *digitaloceanSuite) TestCreateMachinePrivateNetworking(c *check.C) {
	var createRequest map[string]interface{}
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/droplets" {
			err := json.NewDecoder(r.Body).Decode(&createRequest)
			c.Assert(err, check.IsNil)
			fmt.Fprintln(w, `{"droplet": {"id": 1, "status": "new", "networks": {"v4": [], "v6": []}}}`)
		}
		if r.URL.Path == "/v2/droplets/1" {
			fmt.Fprintln(w, `{"droplet": {"id": 1, "status": "active", "networks": {"v4": [{"ip_address": "104.131.186.241", "netmask": "255.255.240.0", "gateway": "104.131.176.1", "type": "public"}, {"ip_address": "10.128.19.90", "netmask": "255.255.240.0", "gateway": "10.128.19.1", "type": "private"}], "v6": [{"ip_address": "2604:A880:0800:0010:0000:0000:031D:2001", "netmask": 64, "gateway": "2604:A880:0800:0010:0000:0000:0000:0001", "type": "public"}]}}}`)
		}
	}))
	defer fakeServer.Close()
	config.Set("iaas:digitalocean:url", fakeServer.URL)
	do := newDigitalOceanIaas("digitalocean")
	params := map[string]string{
		"name":               "example.com",
		"region":             "nyc3",
		"size":               "512mb",
		"image":              "ubuntu-14-04-x64",
		"ssh-keys":           "5050,2032,07:b9:a1:65:1b,13",
		"private-networking": "1",
	}
	m, err := do.CreateMachine(params)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.NotNil)
	c.Assert(m.Address, check.Equals, "10.128.19.90")
	c.Assert(m.Id, check.Equals, "1")
	c.Assert(m.Status, check.Equals, "active")
	expectedKeys := []interface{}{float64(5050), float64(2032), "07:b9:a1:65:1b", float64(13)}
	c.Assert(createRequest["ssh_keys"], check.DeepEquals, expectedKeys)
	c.Assert(createRequest["private_networking"], check.Equals, true)
}

func (s *digitaloceanSuite) TestCreateMachineFailure(c *check.C) {
	config.Set("iaas:digitalocean:wait-timeout", 1)
	defer config.Unset("iaas:digitalocean:wait-timeout")
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"droplet": {"id": 1, "status": "active", "networks": {"v4": [], "v6": []}}}`)
	}))
	defer fakeServer.Close()
	config.Set("iaas:digitalocean:url", fakeServer.URL)
	do := newDigitalOceanIaas("digitalocean")
	params := map[string]string{
		"name":   "example.com",
		"region": "nyc3",
		"size":   "512mb",
		"image":  "ubuntu-14-04-x64",
	}
	_, err := do.CreateMachine(params)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "timed out waiting for machine network")
}

func (s *digitaloceanSuite) TestDeleteMachine(c *check.C) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/droplets/503" {
			w.WriteHeader(204)
			w.Header().Set("Content-type", "application/json")
		}
		if r.URL.Path == "/v2/droplets/503/actions" {
			fmt.Fprintln(w, `{"action":{"id": 123456, "status": "in-progress", "started_at": "2014-11-04T17:08:03Z", "resource_id": 503, "resource_type": "droplet"}}`)
		}
		if r.URL.Path == "/v2/actions/123456" {
			fmt.Fprintln(w, `{"action":{"id": 123456, "status": "completed", "started_at": "2014-11-04T17:08:03Z", "resource_id": 503, "resource_type": "droplet"}}`)
		}
	}))
	defer fakeServer.Close()
	config.Set("iaas:digitalocean:url", fakeServer.URL)
	do := newDigitalOceanIaas("digitalocean")
	machine := iaas.Machine{Id: "503", CreationParams: map[string]string{"projectid": "projid"}}
	err := do.DeleteMachine(&machine)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
}

func (s *digitaloceanSuite) TestDeleteMachineFailure(c *check.C) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/droplets/13" {
			w.WriteHeader(200)
			w.Header().Set("Content-type", "application/json")
		}
		if r.URL.Path == "/v2/droplets/13/actions" {
			fmt.Fprintln(w, `{"action":{"id": 123456, "status": "in-progress", "started_at": "2014-11-04T17:08:03Z", "resource_id": 13, "resource_type": "droplet"}}`)
		}
		if r.URL.Path == "/v2/actions/123456" {
			fmt.Fprintln(w, `{"action":{"id": 123456, "status": "completed", "started_at": "2014-11-04T17:08:03Z", "resource_id": 503, "resource_type": "droplet"}}`)
		}
	}))
	defer fakeServer.Close()
	config.Set("iaas:digitalocean:url", fakeServer.URL)
	do := newDigitalOceanIaas("digitalocean")
	machine := iaas.Machine{Id: "13", CreationParams: map[string]string{"projectid": "projid"}}
	err := do.DeleteMachine(&machine)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "failed to delete machine")
}
