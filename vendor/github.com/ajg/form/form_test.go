// Copyright 2014 Alvaro J. Genial. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package form

import (
	"encoding"
	"fmt"
	"net/url"
	"time"
)

type Struct struct {
	B  bool
	I  int `form:"life"`
	F  float64
	C  complex128
	R  rune `form:",omitempty"` // For testing when non-empty.
	Re rune `form:",omitempty"` // For testing when empty.
	S  string
	T  time.Time
	U  url.URL
	A  Array
	M  Map
	Y  interface{} `form:"-"` // For testing when non-empty.
	Ye interface{} `form:"-"` // For testing when empty.
	Zs Slice
	E    // Embedded.
	P  P `form:"P.D\\Q.B"`
}

type SXs map[string]interface{}
type E struct {
	Bytes1 []byte // For testing explicit (qualified by embedder) name, e.g. "E.Bytes1".
	Bytes2 []byte // For testing implicit (unqualified) name, e.g. just "Bytes2"
}

type Z time.Time // Defined as such to test conversions.

func (z Z) String() string { return time.Time(z).String() }

type Array [3]string
type Map map[string]int
type Slice []struct {
	Z  Z
	Q  Q
	Qp *Q
	Q2 Q `form:"-"`
	E  `form:"-"`
}

// Custom marshaling
type Q struct {
	a, b uint16
}

var (
	_ encoding.TextMarshaler   = &Q{}
	_ encoding.TextUnmarshaler = &Q{}
)

func (u Q) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%d_%d", u.a, u.b)), nil
}

func (u *Q) UnmarshalText(bs []byte) error {
	_, err := fmt.Sscanf(string(bs), "%d_%d", &u.a, &u.b)
	return err
}

func prepopulate(sxs SXs) SXs {
	var B bool
	var I int
	var F float64
	var C complex128
	var R rune
	var S string
	var T time.Time
	var U url.URL
	var A Array
	var M Map
	// Y is ignored.
	// Ye is ignored.
	var Zs Slice
	var E E
	var P P
	sxs["B"] = B
	sxs["life"] = I
	sxs["F"] = F
	sxs["C"] = C
	sxs["R"] = R
	// Re is omitted.
	sxs["S"] = S
	sxs["T"] = T
	sxs["U"] = U
	sxs["A"] = A
	sxs["M"] = M
	// Y is ignored.
	// Ye is ignored.
	sxs["Zs"] = Zs
	sxs["E"] = E
	sxs["P.D\\Q.B"] = P
	return sxs
}

type P struct {
	A, B string
}

type direction int

const (
	encOnly = 1
	decOnly = 2
	rndTrip = encOnly | decOnly
)

func testCases(dir direction) (cs []testCase) {
	var B bool
	var I int
	var F float64
	var C complex128
	var R rune
	var S string
	var T time.Time
	var U url.URL
	const canonical = `A.0=x&A.1=y&A.2=z&B=true&C=42%2B6.6i&E.Bytes1=%00%01%02&E.Bytes2=%03%04%05&F=6.6&M.Bar=8&M.Foo=7&M.Qux=9&P%5C.D%5C%5CQ%5C.B.A=P%2FD&P%5C.D%5C%5CQ%5C.B.B=Q-B&R=8734&S=Hello%2C+there.&T=2013-10-01T07%3A05%3A34.000000088Z&U=http%3A%2F%2Fexample.org%2Ffoo%23bar&Zs.0.Q=11_22&Zs.0.Qp=33_44&Zs.0.Z=2006-12-01&life=42`
	const variation = `;C=42%2B6.6i;A.0=x;M.Bar=8;F=6.6;A.1=y;R=8734;A.2=z;Zs.0.Qp=33_44;B=true;M.Foo=7;T=2013-10-01T07:05:34.000000088Z;E.Bytes1=%00%01%02;Bytes2=%03%04%05;Zs.0.Q=11_22;Zs.0.Z=2006-12-01;M.Qux=9;life=42;S=Hello,+there.;P\.D\\Q\.B.A=P/D;P\.D\\Q\.B.B=Q-B;U=http%3A%2F%2Fexample.org%2Ffoo%23bar;`

	for _, c := range []testCase{
		// Bools
		{rndTrip, &B, "=", b(false)},
		{rndTrip, &B, "=true", b(true)},
		{decOnly, &B, "=false", b(false)},

		// Ints
		{rndTrip, &I, "=", i(0)},
		{rndTrip, &I, "=42", i(42)},
		{rndTrip, &I, "=-42", i(-42)},
		{decOnly, &I, "=0", i(0)},
		{decOnly, &I, "=-0", i(0)},

		// Floats
		{rndTrip, &F, "=", f(0)},
		{rndTrip, &F, "=6.6", f(6.6)},
		{rndTrip, &F, "=-6.6", f(-6.6)},

		// Complexes
		{rndTrip, &C, "=", c(complex(0, 0))},
		{rndTrip, &C, "=42%2B6.6i", c(complex(42, 6.6))},
		{rndTrip, &C, "=-42-6.6i", c(complex(-42, -6.6))},

		// Runes
		{rndTrip, &R, "=", r(0)},
		{rndTrip, &R, "=97", r('a')},
		{rndTrip, &R, "=8734", r('\u221E')},

		// Strings
		{rndTrip, &S, "=", s("")},
		{rndTrip, &S, "=X+%26+Y+%26+Z", s("X & Y & Z")},
		{rndTrip, &S, "=Hello%2C+there.", s("Hello, there.")},
		{decOnly, &S, "=Hello, there.", s("Hello, there.")},

		// Dates/Times
		{rndTrip, &T, "=", t(time.Time{})},
		{rndTrip, &T, "=2013-10-01T07%3A05%3A34.000000088Z", t(time.Date(2013, 10, 1, 7, 5, 34, 88, time.UTC))},
		{decOnly, &T, "=2013-10-01T07:05:34.000000088Z", t(time.Date(2013, 10, 1, 7, 5, 34, 88, time.UTC))},
		{rndTrip, &T, "=07%3A05%3A34.000000088Z", t(time.Date(0, 1, 1, 7, 5, 34, 88, time.UTC))},
		{decOnly, &T, "=07:05:34.000000088Z", t(time.Date(0, 1, 1, 7, 5, 34, 88, time.UTC))},
		{rndTrip, &T, "=2013-10-01", t(time.Date(2013, 10, 1, 0, 0, 0, 0, time.UTC))},

		// URLs
		{rndTrip, &U, "=", u(url.URL{})},
		{rndTrip, &U, "=http%3A%2F%2Fexample.org%2Ffoo%23bar", u(url.URL{Scheme: "http", Host: "example.org", Path: "/foo", Fragment: "bar"})},
		{rndTrip, &U, "=git%3A%2F%2Fgithub.com%2Fajg%2Fform.git", u(url.URL{Scheme: "git", Host: "github.com", Path: "/ajg/form.git"})},

		// Structs
		{rndTrip, &Struct{Y: 786}, canonical,
			&Struct{
				true,
				42,
				6.6,
				complex(42, 6.6),
				'\u221E',
				rune(0),
				"Hello, there.",
				time.Date(2013, 10, 1, 7, 5, 34, 88, time.UTC),
				url.URL{Scheme: "http", Host: "example.org", Path: "/foo", Fragment: "bar"},
				Array{"x", "y", "z"},
				Map{"Foo": 7, "Bar": 8, "Qux": 9},
				786, // Y: This value should not change.
				nil, // Ye: This value should not change.
				Slice{{Z(time.Date(2006, 12, 1, 0, 0, 0, 0, time.UTC)), Q{11, 22}, &Q{33, 44}, Q{}, E{}}},
				E{[]byte{0, 1, 2}, []byte{3, 4, 5}},
				P{"P/D", "Q-B"},
			},
		},
		{decOnly, &Struct{Y: 786}, variation,
			&Struct{
				true,
				42,
				6.6,
				complex(42, 6.6),
				'\u221E',
				rune(0),
				"Hello, there.",
				time.Date(2013, 10, 1, 7, 5, 34, 88, time.UTC),
				url.URL{Scheme: "http", Host: "example.org", Path: "/foo", Fragment: "bar"},
				Array{"x", "y", "z"},
				Map{"Foo": 7, "Bar": 8, "Qux": 9},
				786, // Y: This value should not change.
				nil, // Ye: This value should not change.
				Slice{{Z(time.Date(2006, 12, 1, 0, 0, 0, 0, time.UTC)), Q{11, 22}, &Q{33, 44}, Q{}, E{}}},
				E{[]byte{0, 1, 2}, []byte{3, 4, 5}},
				P{"P/D", "Q-B"},
			},
		},

		// Maps
		{rndTrip, prepopulate(SXs{}), canonical,
			SXs{"B": true,
				"life": 42,
				"F":    6.6,
				"C":    complex(42, 6.6),
				"R":    '\u221E',
				// Re is omitted.
				"S": "Hello, there.",
				"T": time.Date(2013, 10, 1, 7, 5, 34, 88, time.UTC),
				"U": url.URL{Scheme: "http", Host: "example.org", Path: "/foo", Fragment: "bar"},
				"A": Array{"x", "y", "z"},
				"M": Map{"Foo": 7, "Bar": 8, "Qux": 9},
				// Y is ignored.
				// Ye is ignored.
				"Zs":       Slice{{Z(time.Date(2006, 12, 1, 0, 0, 0, 0, time.UTC)), Q{11, 22}, &Q{33, 44}, Q{}, E{}}},
				"E":        E{[]byte{0, 1, 2}, []byte{3, 4, 5}},
				"P.D\\Q.B": P{"P/D", "Q-B"},
			},
		},
		{decOnly, prepopulate(SXs{}), variation,
			SXs{"B": true,
				"life": 42,
				"F":    6.6,
				"C":    complex(42, 6.6),
				"R":    '\u221E',
				// Re is omitted.
				"S": "Hello, there.",
				"T": time.Date(2013, 10, 1, 7, 5, 34, 88, time.UTC),
				"U": url.URL{Scheme: "http", Host: "example.org", Path: "/foo", Fragment: "bar"},
				"A": Array{"x", "y", "z"},
				"M": Map{"Foo": 7, "Bar": 8, "Qux": 9},
				// Y is ignored.
				// Ye is ignored.
				"Zs":       Slice{{Z(time.Date(2006, 12, 1, 0, 0, 0, 0, time.UTC)), Q{11, 22}, &Q{33, 44}, Q{}, E{}}},
				"E":        E{[]byte{0, 1, 2}, nil},
				"Bytes2":   string([]byte{3, 4, 5}),
				"P.D\\Q.B": P{"P/D", "Q-B"},
			},
		},

		{rndTrip, SXs{}, canonical,
			SXs{"B": "true",
				"life": "42",
				"F":    "6.6",
				"C":    "42+6.6i",
				"R":    "8734",
				// Re is omitted.
				"S": "Hello, there.",
				"T": "2013-10-01T07:05:34.000000088Z",
				"U": "http://example.org/foo#bar",
				"A": map[string]interface{}{"0": "x", "1": "y", "2": "z"},
				"M": map[string]interface{}{"Foo": "7", "Bar": "8", "Qux": "9"},
				// Y is ignored.
				// Ye is ignored.
				"Zs": map[string]interface{}{
					"0": map[string]interface{}{
						"Z":  "2006-12-01",
						"Q":  "11_22",
						"Qp": "33_44",
					},
				},
				"E":        map[string]interface{}{"Bytes1": string([]byte{0, 1, 2}), "Bytes2": string([]byte{3, 4, 5})},
				"P.D\\Q.B": map[string]interface{}{"A": "P/D", "B": "Q-B"},
			},
		},
		{decOnly, SXs{}, variation,
			SXs{"B": "true",
				"life": "42",
				"F":    "6.6",
				"C":    "42+6.6i",
				"R":    "8734",
				// Re is omitted.
				"S": "Hello, there.",
				"T": "2013-10-01T07:05:34.000000088Z",
				"U": "http://example.org/foo#bar",
				"A": map[string]interface{}{"0": "x", "1": "y", "2": "z"},
				"M": map[string]interface{}{"Foo": "7", "Bar": "8", "Qux": "9"},
				// Y is ignored.
				// Ye is ignored.
				"Zs": map[string]interface{}{
					"0": map[string]interface{}{
						"Z":  "2006-12-01",
						"Q":  "11_22",
						"Qp": "33_44",
					},
				},
				"E":        map[string]interface{}{"Bytes1": string([]byte{0, 1, 2})},
				"Bytes2":   string([]byte{3, 4, 5}),
				"P.D\\Q.B": map[string]interface{}{"A": "P/D", "B": "Q-B"},
			},
		},
	} {
		if c.d&dir != 0 {
			cs = append(cs, c)
		}
	}
	return cs
}

type testCase struct {
	d direction
	a interface{}
	s string
	b interface{}
}

func b(b bool) *bool             { return &b }
func i(i int) *int               { return &i }
func f(f float64) *float64       { return &f }
func c(c complex128) *complex128 { return &c }
func r(r rune) *rune             { return &r }
func s(s string) *string         { return &s }
func t(t time.Time) *time.Time   { return &t }
func u(u url.URL) *url.URL       { return &u }

func mustParseQuery(s string) url.Values {
	vs, err := url.ParseQuery(s)
	if err != nil {
		panic(err)
	}
	return vs
}
