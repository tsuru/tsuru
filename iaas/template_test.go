// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import "launchpad.net/gocheck"

func (s *S) TestTemplateSave(c *gocheck.C) {
	t := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Name, gocheck.Equals, "tpl1")
	c.Assert(t.IaaSName, gocheck.Equals, "test-iaas")
	c.Assert(t.Data, gocheck.DeepEquals, TemplateDataList{
		{Name: "key1", Value: "val1"},
		{Name: "key2", Value: "val2"},
	})
}

func (s *S) TestTemplateSaveInvalidIaaS(c *gocheck.C) {
	t := Template{
		Name:     "tpl1",
		IaaSName: "something",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, gocheck.ErrorMatches, ".*something.*not registered")
}

func (s *S) TestTemplateSaveEmptyName(c *gocheck.C) {
	t := Template{
		Name:     "",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, gocheck.ErrorMatches, "template name cannot be empty")
}

func (s *S) TestFindTemplate(c *gocheck.C) {
	tpl1 := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := tpl1.Save()
	c.Assert(err, gocheck.IsNil)
	t, err := FindTemplate("tpl1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Name, gocheck.Equals, "tpl1")
	c.Assert(t.IaaSName, gocheck.Equals, "test-iaas")
	c.Assert(t.Data, gocheck.DeepEquals, TemplateDataList{
		{Name: "key1", Value: "val1"},
		{Name: "key2", Value: "val2"},
	})
}

func (s *S) TestParamsMap(c *gocheck.C) {
	t := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, gocheck.IsNil)
	params := t.paramsMap()
	c.Assert(params, gocheck.DeepEquals, map[string]string{
		"key1": "val1",
		"key2": "val2",
		"iaas": "test-iaas",
	})
}

func (s *S) TestListTemplates(c *gocheck.C) {
	tpl1 := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := tpl1.Save()
	c.Assert(err, gocheck.IsNil)
	tpl2 := Template{
		Name:     "tpl2",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val9"},
			{Name: "key2", Value: "val8"},
		},
	}
	err = tpl2.Save()
	c.Assert(err, gocheck.IsNil)
	templates, err := ListTemplates()
	c.Assert(err, gocheck.IsNil)
	c.Assert(templates, gocheck.DeepEquals, []Template{tpl1, tpl2})
}

func (s *S) TestDestroyTemplate(c *gocheck.C) {
	t := Template{
		Name:     "tpl1",
		IaaSName: "test-iaas",
		Data: TemplateDataList{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		},
	}
	err := t.Save()
	c.Assert(err, gocheck.IsNil)
	err = DestroyTemplate("tpl1")
	c.Assert(err, gocheck.IsNil)
	templates, err := ListTemplates()
	c.Assert(err, gocheck.IsNil)
	c.Assert(templates, gocheck.HasLen, 0)
}
