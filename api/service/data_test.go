package service

import (
	. "launchpad.net/gocheck"
	"sort"
)

func (s *S) TestAddItem(c *C) {
	set := NewSet()
	set.Add("abc")
	c.Assert(set.m["abc"], Equals, int8(1))
}

func (s *S) TestRemoveItem(c *C) {
	set := NewSet()
	set.Add("abc")
	set.Add("def")
	set.Remove("def")
	_, ok := set.m["def"]
	c.Assert(ok, Equals, false)
}

func (s *S) TestGetItems(c *C) {
	set := NewSet()
	set.Add("abc")
	set.Add("def")
	got := set.Items()
	sort.Strings(got)
	c.Assert(got, DeepEquals, []string{"abc", "def"})
}
