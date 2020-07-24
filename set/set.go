// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package set

import (
	"reflect"
	"sort"
)

var none struct{}

type Set map[string]struct{}

func (s Set) Add(value ...string) {
	for _, v := range value {
		s[v] = none
	}
}

func (s Set) Includes(v string) bool {
	_, ok := s[v]
	return ok
}

func (s Set) Intersection(other Set) Set {
	if len(s) == 0 {
		return other
	}
	if len(other) == 0 {
		return s
	}
	newSet := Set{}
	for key := range s {
		if _, in := other[key]; in {
			newSet.Add(key)
		}
	}
	return newSet
}

func (s Set) Difference(other Set) Set {
	newSet := Set{}
	for key := range s {
		if _, in := other[key]; !in {
			newSet[key] = struct{}{}
		}
	}
	return newSet
}

func (s Set) Sorted() []string {
	result := make([]string, len(s))
	i := 0
	for key := range s {
		result[i] = key
		i++
	}
	sort.Strings(result)
	return result
}

func (s Set) Equal(other Set) bool {
	if len(s) != len(other) {
		return false
	}
	return len(s.Intersection(other)) == len(s)
}

func FromValues(l ...string) Set {
	return FromSlice(l)
}

func FromSlice(l []string) Set {
	s := Set{}
	for _, v := range l {
		s[v] = struct{}{}
	}
	return s
}

func FromMap(m interface{}) Set {
	s := Set{}
	v := reflect.ValueOf(m)
	if v.Kind() != reflect.Map {
		return s
	}
	for _, k := range v.MapKeys() {
		s[k.String()] = struct{}{}
	}
	return s
}
