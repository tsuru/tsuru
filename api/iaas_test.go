// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/iaas"
	check "gopkg.in/check.v1"
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
	if params["pool"] != "" {
		m.Id += "-" + params["pool"]
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
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
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
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeIaas, Value: "test-iaas"},
		Owner:  s.token.GetUserName(),
		Kind:   "machine.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":machine_id", "value": "myid1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestMachinesDestroyError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/iaas/machines/myid1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
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
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
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
	defer iaas.DestroyTemplate("my-tpl")
	v, err := form.EncodeToValues(&data)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/iaas/templates", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	templates, err := iaas.ListTemplates()
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.HasLen, 1)
	c.Assert(templates[0].Name, check.Equals, "my-tpl")
	c.Assert(templates[0].IaaSName, check.Equals, "my-iaas")
	c.Assert(templates[0].Data, check.DeepEquals, iaas.TemplateDataList([]iaas.TemplateData{
		{Name: "x", Value: "y"},
		{Name: "a", Value: "b"},
	}))
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeIaas, Value: "my-iaas"},
		Owner:  s.token.GetUserName(),
		Kind:   "machine.template.create",
		StartCustomData: []map[string]interface{}{
			{"name": "Name", "value": "my-tpl"},
			{"name": "IaaSName", "value": "my-iaas"},
			{"name": "Data.0.Name", "value": "x"},
			{"name": "Data.0.Value", "value": "y"},
			{"name": "Data.1.Name", "value": "a"},
			{"name": "Data.1.Value", "value": "b"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestTemplateCreateAlreadyExists(c *check.C) {
	iaas.RegisterIaasProvider("my-iaas", newTestIaaS)
	iaas.RegisterIaasProvider("ec2", newTestIaaS)
	data := iaas.Template{
		Name:     "my-tpl",
		IaaSName: "my-iaas",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "x", Value: "y"},
			{Name: "a", Value: "b"},
		}),
	}
	err := data.Save()
	newTemplate := iaas.Template{
		Name:     "my-tpl",
		IaaSName: "ec2",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "b", Value: "a"},
		}),
	}
	defer iaas.DestroyTemplate("my-tpl")
	c.Assert(err, check.IsNil)
	v, err := form.EncodeToValues(&newTemplate)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/iaas/templates", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, "template name \"my-tpl\" already used\n")
}

func (s *S) TestTemplateCreateBadRequest(c *check.C) {
	iaas.RegisterIaasProvider("my-iaas", newTestIaaS)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/iaas/templates", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
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
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	templates, err := iaas.ListTemplates()
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeIaas, Value: "ec2"},
		Owner:  s.token.GetUserName(),
		Kind:   "machine.template.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":template_name", "value": "tpl1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestTemplateUpdate(c *check.C) {
	iaas.RegisterIaasProvider("my-iaas", newTestIaaS)
	tpl1 := iaas.Template{
		Name:     "my-tpl",
		IaaSName: "my-iaas",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "x", Value: "y"},
			{Name: "a", Value: "b"},
		}),
	}
	err := tpl1.Save()
	defer iaas.DestroyTemplate("my-tpl")
	c.Assert(err, check.IsNil)
	tplParam := iaas.Template{
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "x", Value: ""},
			{Name: "y", Value: "8"},
			{Name: "z", Value: "9"},
		}),
	}
	v, err := form.EncodeToValues(&tplParam)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/iaas/templates/my-tpl", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	templates, err := iaas.ListTemplates()
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.HasLen, 1)
	c.Assert(templates[0].Name, check.Equals, "my-tpl")
	c.Assert(templates[0].IaaSName, check.Equals, "my-iaas")
	sort.Sort(templates[0].Data)
	c.Assert(templates[0].Data, check.DeepEquals, iaas.TemplateDataList([]iaas.TemplateData{
		{Name: "a", Value: "b"},
		{Name: "y", Value: "8"},
		{Name: "z", Value: "9"},
	}))
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeIaas, Value: "my-iaas"},
		Owner:  s.token.GetUserName(),
		Kind:   "machine.template.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":template_name", "value": "my-tpl"},
			{"name": "Data.0.Name", "value": "x"},
			{"name": "Data.0.Value", "value": ""},
			{"name": "Data.1.Name", "value": "y"},
			{"name": "Data.1.Value", "value": "8"},
			{"name": "Data.2.Name", "value": "z"},
			{"name": "Data.2.Value", "value": "9"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestTemplateUpdateNotFound(c *check.C) {
	iaas.RegisterIaasProvider("my-iaas", newTestIaaS)
	tplParam := iaas.Template{
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "x", Value: ""},
			{Name: "y", Value: "8"},
			{Name: "z", Value: "9"},
		}),
	}
	v, err := form.EncodeToValues(&tplParam)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/iaas/templates/my-tpl", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "template not found\n")
}

func (s *S) TestTemplateUpdateBadRequest(c *check.C) {
	iaas.RegisterIaasProvider("my-iaas", newTestIaaS)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/iaas/templates/my-tpl", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestTemplateUpdateIaasName(c *check.C) {
	iaas.RegisterIaasProvider("my-iaas", newTestIaaS)
	iaas.RegisterIaasProvider("ec2", newTestIaaS)
	tpl1 := iaas.Template{
		Name:     "my-tpl",
		IaaSName: "my-iaas",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "a", Value: "b"},
		}),
	}
	err := tpl1.Save()
	c.Assert(err, check.IsNil)
	defer iaas.DestroyTemplate("my-tpl")
	tplParam := iaas.Template{
		IaaSName: "ec2",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "a", Value: "c"},
		}),
	}
	v, err := form.EncodeToValues(&tplParam)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/iaas/templates/my-tpl", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	templates, err := iaas.ListTemplates()
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.HasLen, 1)
	c.Assert(templates[0].Name, check.Equals, "my-tpl")
	c.Assert(templates[0].IaaSName, check.Equals, "ec2")
	sort.Sort(templates[0].Data)
	c.Assert(templates[0].Data, check.DeepEquals, iaas.TemplateDataList([]iaas.TemplateData{
		{Name: "a", Value: "c"},
	}))
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeIaas, Value: "ec2"},
		Owner:  s.token.GetUserName(),
		Kind:   "machine.template.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":template_name", "value": "my-tpl"},
			{"name": "Data.0.Name", "value": "a"},
			{"name": "Data.0.Value", "value": "c"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestTemplateUpdateNotRegistered(c *check.C) {
	iaas.RegisterIaasProvider("my-iaas", newTestIaaS)
	tpl1 := iaas.Template{
		Name:     "my-tpl",
		IaaSName: "my-iaas",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "a", Value: "b"},
		}),
	}
	err := tpl1.Save()
	c.Assert(err, check.IsNil)
	defer iaas.DestroyTemplate("my-tpl")
	tplParam := iaas.Template{
		IaaSName: "not-registered",
		Data: iaas.TemplateDataList([]iaas.TemplateData{
			{Name: "a", Value: "c"},
		}),
	}
	v, err := form.EncodeToValues(&tplParam)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/iaas/templates/my-tpl", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "IaaS provider \"not-registered\" based on \"not-registered\" not registered\n")
}
