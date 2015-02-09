// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"sort"

	"gopkg.in/check.v1"
)

type SetSuite struct{}

var _ = check.Suite(&SetSuite{})

func (s *SetSuite) TestSet(c *check.C) {
	animals := []string{"dog", "elephant", "snake", "frog"}
	mammals := []string{"dog", "elephant"}
	animalsSet := set{}
	mammalsSet := set{}
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
