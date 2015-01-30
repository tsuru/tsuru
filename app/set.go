// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

var none struct{}

type set map[string]struct{}

func (s set) Add(value ...string) {
	for _, v := range value {
		s[v] = none
	}
}

func (s set) Includes(v string) bool {
	_, ok := s[v]
	return ok
}

func (s set) Intersection(other set) set {
	if len(s) == 0 {
		return other
	}
	if len(other) == 0 {
		return s
	}
	newSet := set{}
	for key := range s {
		if _, in := other[key]; in {
			newSet.Add(key)
		}
	}
	return newSet
}
