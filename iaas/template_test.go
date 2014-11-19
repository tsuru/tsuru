// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"sort"

	"launchpad.net/gocheck"
)

type tplDataList []templateData

func (l tplDataList) Len() int           { return len(l) }
func (l tplDataList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l tplDataList) Less(i, j int) bool { return l[i].Name < l[j].Name }

func (s *S) TestNewTemplate(c *gocheck.C) {
	t, err := NewTemplate("tpl1", "ec2", map[string]string{
		"key1": "val1",
		"key2": "val2",
	})
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Name, gocheck.Equals, "tpl1")
	c.Assert(t.IaaSName, gocheck.Equals, "ec2")
	sort.Sort(tplDataList(t.Data))
	c.Assert(t.Data, gocheck.DeepEquals, []templateData{
		{Name: "key1", Value: "val1"},
		{Name: "key2", Value: "val2"},
	})
}

func (s *S) TestFindTemplate(c *gocheck.C) {
	_, err := NewTemplate("tpl1", "ec2", map[string]string{
		"key1": "val1",
		"key2": "val2",
	})
	c.Assert(err, gocheck.IsNil)
	t, err := FindTemplate("tpl1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Name, gocheck.Equals, "tpl1")
	c.Assert(t.IaaSName, gocheck.Equals, "ec2")
	sort.Sort(tplDataList(t.Data))
	c.Assert(t.Data, gocheck.DeepEquals, []templateData{
		{Name: "key1", Value: "val1"},
		{Name: "key2", Value: "val2"},
	})
}

func (s *S) TestParamsMap(c *gocheck.C) {
	t, err := NewTemplate("tpl1", "ec2", map[string]string{
		"key1": "val1",
		"key2": "val2",
	})
	c.Assert(err, gocheck.IsNil)
	params := t.paramsMap()
	c.Assert(params, gocheck.DeepEquals, map[string]string{
		"key1": "val1",
		"key2": "val2",
		"iaas": "ec2",
	})
}
