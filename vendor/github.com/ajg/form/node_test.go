// Copyright 2014 Alvaro J. Genial. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package form

import (
	"reflect"
	"testing"
)

type foo int
type bar interface {
	void()
}
type qux struct{}
type zee []bar

func TestCanIndexOrdinally(t *testing.T) {
	for _, c := range []struct {
		x interface{}
		b bool
	}{
		{int(0), false},
		{foo(0), false},
		{qux{}, false},
		{(*int)(nil), false},
		{(*foo)(nil), false},
		{(*bar)(nil), false},
		{(*qux)(nil), false},
		{[]qux{}, true},
		{[5]qux{}, true},
		{&[]foo{}, true},
		{&[5]foo{}, true},
		{zee{}, true},
		{&zee{}, true},
		{map[int]foo{}, false},
		{map[string]interface{}{}, false},
		{map[interface{}]bar{}, false},
		{(chan<- int)(nil), false},
		{(chan bar)(nil), false},
		{(<-chan foo)(nil), false},
	} {
		v := reflect.ValueOf(c.x)
		if b := canIndexOrdinally(v); b != c.b {
			t.Errorf("canIndexOrdinally(%#v)\n want (%#v)\n have (%#v)", v, c.b, b)
		}
	}
}

var escapingTestCases = []struct {
	a, b string
	d, e rune
}{
	{"Foo", "Foo", defaultDelimiter, defaultEscape},
	{"Foo", "Foo", '/', '^'},
	{"Foo.Bar.Qux", "Foo\\.Bar\\.Qux", defaultDelimiter, defaultEscape},
	{"Foo.Bar.Qux", "Foo.Bar.Qux", '/', '^'},
	{"Foo/Bar/Qux", "Foo/Bar/Qux", defaultDelimiter, defaultEscape},
	{"Foo/Bar/Qux", "Foo^/Bar^/Qux", '/', '^'},
	{"0", "0", defaultDelimiter, defaultEscape},
	{"0", "0", '/', '^'},
	{"0.1.2", "0\\.1\\.2", defaultDelimiter, defaultEscape},
	{"0.1.2", "0.1.2", '/', '^'},
	{"0/1/2", "0/1/2", defaultDelimiter, defaultEscape},
	{"0/1/2", "0^/1^/2", '/', '^'},
	{"A\\B", "A\\\\B", defaultDelimiter, defaultEscape},
	{"A\\B", "A\\B", '/', '^'},
	{"A^B", "A^B", defaultDelimiter, defaultEscape},
	{"A^B", "A^^B", '/', '^'},
}

func TestEscape(t *testing.T) {
	for _, c := range escapingTestCases {
		if b := escape(c.d, c.e, c.a); b != c.b {
			t.Errorf("escape(%q, %q, %q)\n want (%#v)\n have (%#v)", c.d, c.e, c.a, c.b, b)
		}
	}
}

func TestUnescape(t *testing.T) {
	for _, c := range escapingTestCases {
		if a := unescape(c.d, c.e, c.b); a != c.a {
			t.Errorf("unescape(%q, %q, %q)\n want (%#v)\n have (%#v)", c.d, c.e, c.b, c.a, a)
		}
	}
}
