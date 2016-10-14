// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package set

import (
	"sort"
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type SetSuite struct{}

var _ = check.Suite(&SetSuite{})

func (s *SetSuite) TestSetIntersection(c *check.C) {
	animals := []string{"dog", "elephant", "snake", "frog"}
	mammals := []string{"dog", "elephant"}
	animalsSet := Set{}
	mammalsSet := Set{}
	for _, animal := range animals {
		animalsSet.Add(animal)
	}
	for _, animal := range mammals {
		mammalsSet.Add(animal)
	}
	intersection := animalsSet.Intersection(mammalsSet)
	result := []string{}
	for key := range intersection {
		result = append(result, key)
	}
	expected := []string{"dog", "elephant"}
	sort.Strings(expected)
	sort.Strings(result)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *SetSuite) TestSetDiff(c *check.C) {
	s1 := FromValues("a", "b", "c")
	s2 := FromValues("b", "c", "d")
	diff := s1.Difference(s2)
	c.Assert(diff, check.DeepEquals, FromValues("a"))
	diff = s2.Difference(s1)
	c.Assert(diff, check.DeepEquals, FromValues("d"))
}
