// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"strings"
)

type MapFlag map[string]string

func (f *MapFlag) String() string {
	repr := *f
	if repr == nil {
		repr = MapFlag{}
	}
	data, _ := json.Marshal(repr)
	return string(data)
}

func (f *MapFlag) Set(val string) error {
	parts := strings.SplitN(val, "=", 2)
	if *f == nil {
		*f = map[string]string{}
	}
	(*f)[parts[0]] = parts[1]
	return nil
}

type StringSliceFlag []string

func (f *StringSliceFlag) String() string {
	repr := *f
	if repr == nil {
		repr = StringSliceFlag{}
	}
	data, _ := json.Marshal(repr)
	return string(data)
}

func (f *StringSliceFlag) Set(val string) error {
	*f = append(*f, val)
	return nil
}
