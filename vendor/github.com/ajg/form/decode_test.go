// Copyright 2014 Alvaro J. Genial. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package form

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeString(t *testing.T) {
	for _, c := range testCases(decOnly) {
		if err := DecodeString(c.a, c.s); err != nil {
			t.Errorf("DecodeString(%#v): %s", c.s, err)
		} else if !reflect.DeepEqual(c.a, c.b) {
			t.Errorf("DecodeString(%#v)\n want (%#v)\n have (%#v)", c.s, c.b, c.a)
		}
	}
}

func TestDecodeValues(t *testing.T) {
	for _, c := range testCases(decOnly) {
		vs := mustParseQuery(c.s)

		if err := DecodeValues(c.a, vs); err != nil {
			t.Errorf("DecodeValues(%#v): %s", vs, err)
		} else if !reflect.DeepEqual(c.a, c.b) {
			t.Errorf("DecodeValues(%#v)\n want (%#v)\n have (%#v)", vs, c.b, c.a)
		}
	}
}

func TestDecode(t *testing.T) {
	for _, c := range testCases(decOnly) {
		r := strings.NewReader(c.s)
		d := NewDecoder(r)

		if err := d.Decode(c.a); err != nil {
			t.Errorf("Decode(%#v): %s", r, err)
		} else if !reflect.DeepEqual(c.a, c.b) {
			t.Errorf("Decode(%#v)\n want (%#v)\n have (%#v)", r, c.b, c.a)
		}
	}
}

func TestDecodeIgnoreUnknown(t *testing.T) {
	type simpleStruct struct{ A string }
	var dst simpleStruct
	values := url.Values{
		"b": []string{"2"},
		"A": []string{"1"},
	}
	expected := simpleStruct{A: "1"}
	d := NewDecoder(nil)
	err := d.DecodeValues(&dst, values)
	if err == nil || err.Error() != "b doesn't exist in form.simpleStruct" {
		t.Errorf("Decode(%#v): expected error got nil", values)
	}
	d.IgnoreUnknownKeys(true)
	err = d.DecodeValues(&dst, values)
	if err != nil {
		t.Errorf("Decode(%#v): %s", values, err)
	}
	if !reflect.DeepEqual(dst, expected) {
		t.Errorf("Decode(%#v)\n want (%#v)\n have (%#v)", values, expected, dst)
	}
}

func TestDecodeIgnoreCase(t *testing.T) {
	type simpleStruct struct{ AaAA string }
	var dst simpleStruct
	values := url.Values{
		"aAaA": []string{"1"},
	}
	expected := simpleStruct{AaAA: "1"}
	d := NewDecoder(nil)
	err := d.DecodeValues(&dst, values)
	if err == nil || err.Error() != "aAaA doesn't exist in form.simpleStruct" {
		t.Errorf("Decode(%#v): expected error got nil", values)
	}
	d.IgnoreCase(true)
	err = d.DecodeValues(&dst, values)
	if err != nil {
		t.Errorf("Decode(%#v): %s", values, err)
	}
	if !reflect.DeepEqual(dst, expected) {
		t.Errorf("Decode(%#v)\n want (%#v)\n have (%#v)", values, expected, dst)
	}
}

func TestDecodeIgnoreCasePriority(t *testing.T) {
	type simpleStruct struct {
		Aaa string
		AaA string
		AAA string
	}
	var dst simpleStruct
	values := url.Values{
		"AaA": []string{"1"},
	}
	expected := simpleStruct{AaA: "1"}
	d := NewDecoder(nil)
	d.IgnoreCase(true)
	err := d.DecodeValues(&dst, values)
	if err != nil {
		t.Errorf("Decode(%#v): %s", values, err)
	}
	if !reflect.DeepEqual(dst, expected) {
		t.Errorf("Decode(%#v)\n want (%#v)\n have (%#v)", values, expected, dst)
	}
}
