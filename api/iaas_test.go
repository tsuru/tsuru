// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/iaas"
	"gopkg.in/check.v1"
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

func newTestIaaS(string) iaas.IaaS {
	return TestIaaS{}
}

func (s *S) TestMachinesList(c *check.C) {
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	_, err := iaas.CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	defer (&iaas.Machine{Id: "myid1"}).Destroy()
	c.Assert(err, check.IsNil)
	_, err = iaas.CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid2"})
	defer (&iaas.Machine{Id: "myid2"}).Destroy()
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/iaas/machines", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var machines []iaas.Machine
	err = json.NewDecoder(recorder.Body).Decode(&machines)
	c.Assert(err, check.IsNil)
	c.Assert(machines[0].Id, check.Equals, "myid1")
	c.Assert(machines[0].Address, check.Equals, "myid1.somewhere.com")
	c.Assert(machines[0].CreationParams, check.DeepEquals, map[string]string{
		"id":      "myid1",
		"iaas":    "test-iaas",
		"iaas-id": "myid1",
	})
	c.Assert(machines[1].Id, check.Equals, "myid2")
	c.Assert(machines[1].Address, check.Equals, "myid2.somewhere.com")
	c.Assert(machines[1].CreationParams, check.DeepEquals, map[string]string{
		"id":      "myid2",
		"iaas":    "test-iaas",
		"iaas-id": "myid2",
	})
}

func (s *S) TestMachinesDestroy(c *check.C) {
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	_, err := iaas.CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/iaas/machines/myid1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestMachinesDestroyError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/iaas/machines/myid1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "machine not found\n")
}

func (s *S) TestTemplateList(c *check.C) {
	iaas.RegisterIaasProvider("ec2", newTestIaaS)
	iaas.RegisterIaasProvider("other", newTestIaaS)
	tpl1 := iaas.Template{
		Name:     "tpl1",
		IaaSName: "ec2",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		}),
	}
	err := tpl1.Save()
	c.Assert(err, check.IsNil)
	defer iaas.DestroyTemplate("tpl1")
	tpl2 := iaas.Template{
		Name:     "tpl2",
		IaaSName: "other",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "key4", Value: "valX"},
			{Name: "key5", Value: "valY"},
		}),
	}
	err = tpl2.Save()
	c.Assert(err, check.IsNil)
	defer iaas.DestroyTemplate("tpl2")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/iaas/templates", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var templates []iaas.Template
	err = json.Unmarshal(recorder.Body.Bytes(), &templates)
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.HasLen, 2)
	c.Assert(templates[0].Name, check.Equals, "tpl1")
	c.Assert(templates[1].Name, check.Equals, "tpl2")
	c.Assert(templates[0].IaaSName, check.Equals, "ec2")
	c.Assert(templates[1].IaaSName, check.Equals, "other")
	c.Assert(templates[0].Data, check.DeepEquals, iaas.TemplateDataList([]iaas.TemplateData{
		{Name: "key1", Value: "val1"},
		{Name: "key2", Value: "val2"},
	}))
	c.Assert(templates[1].Data, check.DeepEquals, iaas.TemplateDataList([]iaas.TemplateData{
		{Name: "key4", Value: "valX"},
		{Name: "key5", Value: "valY"},
	}))
}

func (s *S) TestTemplateCreate(c *check.C) {
	iaas.RegisterIaasProvider("my-iaas", newTestIaaS)
	data := iaas.Template{
		Name:     "my-tpl",
		IaaSName: "my-iaas",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "x", Value: "y"},
			{Name: "a", Value: "b"},
		}),
	}
	bodyData, err := json.Marshal(data)
	c.Assert(err, check.IsNil)
	body := bytes.NewBuffer(bodyData)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/iaas/templates", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	defer iaas.DestroyTemplate("my-tpl")
	templates, err := iaas.ListTemplates()
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.HasLen, 1)
	c.Assert(templates[0].Name, check.Equals, "my-tpl")
	c.Assert(templates[0].IaaSName, check.Equals, "my-iaas")
	c.Assert(templates[0].Data, check.DeepEquals, iaas.TemplateDataList([]iaas.TemplateData{
		{Name: "x", Value: "y"},
		{Name: "a", Value: "b"},
	}))
}

func (s *S) TestTemplateDestroy(c *check.C) {
	iaas.RegisterIaasProvider("ec2", newTestIaaS)
	tpl1 := iaas.Template{
		Name:     "tpl1",
		IaaSName: "ec2",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		}),
	}
	err := tpl1.Save()
	c.Assert(err, check.IsNil)
	defer iaas.DestroyTemplate("tpl1")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/iaas/templates/tpl1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	templates, err := iaas.ListTemplates()
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.HasLen, 0)
}
