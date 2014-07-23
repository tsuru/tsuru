// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"github.com/tsuru/tsuru/iaas"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type TestIaaS struct{}

func (TestIaaS) DeleteMachine(m *iaas.Machine) error {
	m.Status = "destroyed"
	return nil
}

func (TestIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	m := iaas.Machine{
		Id:      params["id"],
		Status:  "running",
		Address: params["id"] + ".somewhere.com",
	}
	return &m, nil
}

func (s *S) TestMachinesList(c *gocheck.C) {
	iaas.RegisterIaasProvider("test-iaas", TestIaaS{})
	_, err := iaas.CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	defer (&iaas.Machine{Id: "myid1"}).Destroy()
	c.Assert(err, gocheck.IsNil)
	_, err = iaas.CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid2"})
	defer (&iaas.Machine{Id: "myid2"}).Destroy()
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/iaas/machines", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	var machines []iaas.Machine
	err = json.NewDecoder(recorder.Body).Decode(&machines)
	c.Assert(err, gocheck.IsNil)
	c.Assert(machines[0].Id, gocheck.Equals, "myid1")
	c.Assert(machines[0].Address, gocheck.Equals, "myid1.somewhere.com")
	c.Assert(machines[0].CreationParams, gocheck.DeepEquals, map[string]string{
		"id": "myid1",
	})
	c.Assert(machines[1].Id, gocheck.Equals, "myid2")
	c.Assert(machines[1].Address, gocheck.Equals, "myid2.somewhere.com")
	c.Assert(machines[1].CreationParams, gocheck.DeepEquals, map[string]string{
		"id": "myid2",
	})
}

func (s *S) TestMachinesDestroy(c *gocheck.C) {
	iaas.RegisterIaasProvider("test-iaas", TestIaaS{})
	_, err := iaas.CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/iaas/machines/myid1", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
}

func (s *S) TestMachinesDestroyError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/iaas/machines/myid1", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), gocheck.Equals, "machine not found\n")
}
