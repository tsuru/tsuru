// Copyright 2014 Alvaro J. Genial. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package form

import (
	"bytes"
	"reflect"
	"testing"
)

func TestEncodeToString(t *testing.T) {
	for _, c := range testCases(encOnly) {
		if s, err := EncodeToString(c.b); err != nil {
			t.Errorf("EncodeToString(%#v): %s", c.b, err)
		} else if !reflect.DeepEqual(c.s, s) {
			t.Errorf("EncodeToString(%#v)\n want (%#v)\n have (%#v)", c.b, c.s, s)
		}
	}
}

func TestEncodeToValues(t *testing.T) {
	for _, c := range testCases(encOnly) {
		cvs := mustParseQuery(c.s)
		if vs, err := EncodeToValues(c.b); err != nil {
			t.Errorf("EncodeToValues(%#v): %s", c.b, err)
		} else if !reflect.DeepEqual(cvs, vs) {
			t.Errorf("EncodeToValues(%#v)\n want (%#v)\n have (%#v)", c.b, cvs, vs)
		}
	}
}

func TestEncode(t *testing.T) {
	for _, c := range testCases(encOnly) {
		var w bytes.Buffer
		e := NewEncoder(&w)

		if err := e.Encode(c.b); err != nil {
			t.Errorf("Encode(%#v): %s", c.b, err)
		} else if s := w.String(); !reflect.DeepEqual(c.s, s) {
			t.Errorf("Encode(%#v)\n want (%#v)\n have (%#v)", c.b, c.s, s)
		}
	}
}

type Thing1 struct {
	String  string `form:"name,omitempty"`
	Integer *uint  `form:"num,omitempty"`
}

type Thing2 struct {
	String  string `form:"name,omitempty"`
	Integer uint   `form:"num,omitempty"`
}

type Thing3 struct {
	String  string `form:"name"`
	Integer *uint  `form:"num"`
}

type Thing4 struct {
	String  string `form:"name"`
	Integer uint   `form:"num"`
}

func TestEncode_KeepZero(t *testing.T) {
	num := uint(0)
	for _, c := range []struct {
		b interface{}
		s string
		z bool
	}{
		{Thing1{"test", &num}, "name=test&num=", false},
		{Thing1{"test", &num}, "name=test&num=0", true},
		{Thing2{"test", num}, "name=test", false},
		{Thing2{"test", num}, "name=test", true},
		{Thing3{"test", &num}, "name=test&num=", false},
		{Thing3{"test", &num}, "name=test&num=0", true},
		{Thing4{"test", num}, "name=test&num=", false},
		{Thing4{"test", num}, "name=test&num=0", true},
		{Thing1{"", &num}, "num=", false},
		{Thing1{"", &num}, "num=0", true},
		{Thing2{"", num}, "", false},
		{Thing2{"", num}, "", true},
		{Thing3{"", &num}, "name=&num=", false},
		{Thing3{"", &num}, "name=&num=0", true},
		{Thing4{"", num}, "name=&num=", false},
		{Thing4{"", num}, "name=&num=0", true},
	} {

		var w bytes.Buffer
		e := NewEncoder(&w)

		if err := e.KeepZeros(c.z).Encode(c.b); err != nil {
			t.Errorf("KeepZeros(%#v).Encode(%#v): %s", c.z, c.b, err)
		} else if s := w.String(); c.s != s {
			t.Errorf("KeepZeros(%#v).Encode(%#v)\n want (%#v)\n have (%#v)", c.z, c.b, c.s, s)
		}
	}
}
