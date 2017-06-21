// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"errors"
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
	if len(parts) < 2 {
		return errors.New("must be on the form \"key=value\"")
	}
	(*f)[parts[0]] = parts[1]
	return nil
}

type MapFlagWrapper struct {
	Dst *map[string]string
}

func (f MapFlagWrapper) String() string {
	m := MapFlag(*f.Dst)
	return m.String()
}

func (f MapFlagWrapper) Set(val string) error {
	parts := strings.SplitN(val, "=", 2)
	if *f.Dst == nil {
		*f.Dst = map[string]string{}
	}
	if len(parts) < 2 {
		return errors.New("must be on the form \"key=value\"")
	}
	(*f.Dst)[parts[0]] = parts[1]
	return nil
}

type StringSliceFlagWrapper struct {
	Dst *[]string
}

func (f StringSliceFlagWrapper) String() string {
	s := StringSliceFlag(*f.Dst)
	return s.String()
}

func (f StringSliceFlagWrapper) Set(val string) error {
	*f.Dst = append(*f.Dst, val)
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
