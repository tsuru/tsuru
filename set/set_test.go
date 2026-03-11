// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package set

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetIntersection(t *testing.T) {
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
	require.Equal(t, expected, result)
}

func TestSetDiff(t *testing.T) {
	s1 := FromValues("a", "b", "c")
	s2 := FromValues("b", "c", "d")
	diff := s1.Difference(s2)
	require.Equal(t, FromValues("a"), diff)
	diff = s2.Difference(s1)
	require.Equal(t, FromValues("d"), diff)
}

func TestFromMap(t *testing.T) {
	set := FromMap(map[string]string{"a": "1", "b": "2"})
	require.Equal(t, FromValues("a", "b"), set)
	set = FromMap(map[string]int{"a": 1, "b": 2})
	require.Equal(t, FromValues("a", "b"), set)
}
