// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"sort"

	"gopkg.in/check.v1"
)

func (s *S) TestTemplateSave(c *check.C) {
	t := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, check.IsNil)
	c.Assert(t.Name, check.Equals, "tpl1")
	c.Assert(t.IaaSName, check.Equals, "test-iaas")
	c.Assert(t.Data, check.DeepEquals, TemplateDataList{
		{Name: "key1", Value: "val1"},
		{Name: "key2", Value: "val2"},
	})
}

func (s *S) TestTemplateSaveInvalidIaaS(c *check.C) {
	t := Template{
		Name:     "tpl1",
		IaaSName: "something",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, check.ErrorMatches, ".*something.*not registered")
}

func (s *S) TestTemplateSaveEmptyName(c *check.C) {
	t := Template{
		Name:     "",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, check.ErrorMatches, "template name cannot be empty")
}

func (s *S) TestFindTemplate(c *check.C) {
	tpl1 := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := tpl1.Save()
	c.Assert(err, check.IsNil)
	t, err := FindTemplate("tpl1")
	c.Assert(err, check.IsNil)
	c.Assert(t.Name, check.Equals, "tpl1")
	c.Assert(t.IaaSName, check.Equals, "test-iaas")
	c.Assert(t.Data, check.DeepEquals, TemplateDataList{
		{Name: "key1", Value: "val1"},
		{Name: "key2", Value: "val2"},
	})
}

func (s *S) TestUpdateTemplate(c *check.C) {
	tpl1 := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := tpl1.Save()
	c.Assert(err, check.IsNil)
	tpl1.Update(&Template{
		Name:     "ignored",
		IaaSName: "ignored2",
		Data: TemplateDataList{
			{Name: "key3", Value: "val3"},
			{Name: "key2", Value: ""},
		},
	})
	sort.Sort(tpl1.Data)
	c.Assert(tpl1.Data, check.DeepEquals, TemplateDataList{
		{Name: "key1", Value: "val1"},
		{Name: "key3", Value: "val3"},
	})
	dbTpl, err := FindTemplate("tpl1")
	c.Assert(err, check.IsNil)
	c.Assert(dbTpl.Name, check.Equals, "tpl1")
	c.Assert(dbTpl.IaaSName, check.Equals, "test-iaas")
	sort.Sort(dbTpl.Data)
	c.Assert(dbTpl.Data, check.DeepEquals, TemplateDataList{
		{Name: "key1", Value: "val1"},
		{Name: "key3", Value: "val3"},
	})
}

func (s *S) TestParamsMap(c *check.C) {
	t := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, check.IsNil)
	params := t.paramsMap()
	c.Assert(params, check.DeepEquals, map[string]string{
		"key1": "val1",
		"key2": "val2",
		"iaas": "test-iaas",
	})
}

func (s *S) TestListTemplates(c *check.C) {
	tpl1 := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := tpl1.Save()
	c.Assert(err, check.IsNil)
	tpl2 := Template{
		Name:     "tpl2",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val9"},
			{Name: "key2", Value: "val8"},
		},
	}
	err = tpl2.Save()
	c.Assert(err, check.IsNil)
	templates, err := ListTemplates()
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.DeepEquals, []Template{tpl1, tpl2})
}

func (s *S) TestDestroyTemplate(c *check.C) {
	t := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, check.IsNil)
	err = DestroyTemplate("tpl1")
	c.Assert(err, check.IsNil)
	templates, err := ListTemplates()
	c.Assert(err, check.IsNil)
	c.Assert(templates, check.HasLen, 0)
}

func (s *S) TestExpandTemplate(c *check.C) {
	tpl1 := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := tpl1.Save()
	c.Assert(err, check.IsNil)
	params := map[string]string{
		"key2": "myvalue2",
	}
	data, err := ExpandTemplate("tpl1", params)
	c.Assert(err, check.IsNil)
	c.Assert(data, check.DeepEquals, map[string]string{
		"key1": "val1",
		"key2": "myvalue2",
		"iaas": "test-iaas",
	})
}
