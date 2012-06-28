package service

import (
	. "launchpad.net/gocheck"
	"sort"
)

func (s *ServiceSuite) TestAddItem(c *C) {
	set := NewSet()
	set.Add("abc")
	c.Assert(set.m["abc"], Equals, int8(1))
}

func (s *ServiceSuite) TestRemoveItem(c *C) {
	set := NewSet()
	set.Add("abc")
	set.Add("def")
	set.Remove("def")
	_, ok := set.m["def"]
	c.Assert(ok, Equals, false)
}

func (s *ServiceSuite) TestGetItems(c *C) {
	set := NewSet()
	set.Add("abc")
	set.Add("def")
	got := set.Items()
	sort.Strings(got)
	c.Assert(got, DeepEquals, []string{"abc", "def"})
}
