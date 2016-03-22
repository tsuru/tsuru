// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scopedconfig

import (
	"reflect"
	"time"

	"gopkg.in/check.v1"
)

type TestStd struct {
	A  string
	B  string
	C  int
	D  int
	E  []string
	F  []string
	F2 []string
	G  float64
	H  time.Time
	I  time.Time
	Y  TestStdAux
	Z  TestStdAux
}

type TestPtr struct {
	A  *string
	B  *string
	C  *int
	D  *int
	E  []string
	F  []string
	F2 []string
	G  *float64
	H  *time.Time
	I  *time.Time
	Y  *TestStdAux
	Z  *TestStdAux
	Z2 *TestStdAux
}

type TestStdOmit struct {
	A  string     `bson:",omitempty"`
	B  string     `bson:",omitempty"`
	C  int        `bson:",omitempty"`
	D  int        `bson:",omitempty"`
	E  []string   `bson:",omitempty"`
	F  []string   `bson:",omitempty"`
	F2 []string   `bson:",omitempty"`
	G  float64    `bson:",omitempty"`
	H  time.Time  `bson:",omitempty"`
	I  time.Time  `bson:",omitempty"`
	Y  TestStdAux `bson:",omitempty"`
	Z  TestStdAux `bson:",omitempty"`
}

type TestPtrOmit struct {
	A  *string     `bson:",omitempty"`
	B  *string     `bson:",omitempty"`
	C  *int        `bson:",omitempty"`
	D  *int        `bson:",omitempty"`
	E  []string    `bson:",omitempty"`
	F  []string    `bson:",omitempty"`
	F2 []string    `bson:",omitempty"`
	G  *float64    `bson:",omitempty"`
	H  *time.Time  `bson:",omitempty"`
	I  *time.Time  `bson:",omitempty"`
	Y  *TestStdAux `bson:",omitempty"`
	Z  *TestStdAux `bson:",omitempty"`
	Z2 *TestStdAux `bson:",omitempty"`
}

type TestStdAux struct {
	A string
	B string
}

type TestDeepMerge struct {
	Envs map[string]string
	A    TestStdAux
}

func (s *S) TestNScopedConfigMulti(c *check.C) {
	t1 := time.Unix(time.Now().Unix(), 0)
	t2 := t1.Add(time.Minute)
	intPtr := func(a int) *int {
		return &a
	}
	strPtr := func(a string) *string {
		return &a
	}
	floatPtr := func(a float64) *float64 {
		return &a
	}
	tests := []struct {
		base          interface{}
		pool          interface{}
		expected      interface{}
		expectedEmpty interface{}
	}{
		{
			TestStd{
				A:  "",
				B:  "abc",
				C:  222,
				D:  999,
				E:  []string{"A"},
				F:  []string{"B"},
				F2: []string{"C"},
				G:  1.1,
				H:  t1,
				I:  t1,
				Y:  TestStdAux{A: "zzz"},
				Z:  TestStdAux{},
			}, TestStd{
				A:  "x",
				B:  "",
				C:  333,
				D:  0,
				E:  []string{},
				F:  []string{"F"},
				F2: nil,
				I:  t2,
				Z:  TestStdAux{B: "aaa"},
			}, TestStd{
				A:  "x",
				B:  "abc",
				C:  333,
				D:  999,
				E:  []string{"A"},
				F:  []string{"F"},
				F2: []string{"C"},
				G:  1.1,
				H:  t1,
				I:  t2,
				Y:  TestStdAux{A: "zzz"},
				Z:  TestStdAux{B: "aaa"},
			}, TestStd{
				A:  "x",
				B:  "",
				C:  333,
				D:  0,
				E:  []string{},
				F:  []string{"F"},
				F2: []string{},
				G:  0,
				H:  time.Time{},
				I:  t2,
				Y:  TestStdAux{},
				Z:  TestStdAux{B: "aaa"},
			},
		},
		{
			TestStdOmit{
				A:  "",
				B:  "abc",
				C:  222,
				D:  999,
				E:  []string{"A"},
				F:  []string{"B"},
				F2: []string{"C"},
				G:  1.1,
				H:  t1,
				I:  t1,
				Y:  TestStdAux{A: "zzz"},
				Z:  TestStdAux{},
			}, TestStdOmit{
				A:  "x",
				B:  "",
				C:  333,
				D:  0,
				E:  []string{},
				F:  []string{"F"},
				F2: nil,
				I:  t2,
				Z:  TestStdAux{B: "aaa"},
			}, TestStdOmit{
				A:  "x",
				B:  "abc",
				C:  333,
				D:  999,
				E:  []string{"A"},
				F:  []string{"F"},
				F2: []string{"C"},
				G:  1.1,
				H:  t1,
				I:  t2,
				Y:  TestStdAux{A: "zzz"},
				Z:  TestStdAux{B: "aaa"},
			}, TestStdOmit{
				A:  "x",
				B:  "",
				C:  333,
				D:  0,
				E:  []string{"A"},
				F:  []string{"F"},
				F2: []string{"C"},
				G:  0,
				H:  time.Time{},
				I:  t2,
				Y:  TestStdAux{},
				Z:  TestStdAux{B: "aaa"},
			},
		},
		{
			TestPtr{
				A:  strPtr(""),
				B:  strPtr("abc"),
				C:  intPtr(222),
				D:  intPtr(999),
				E:  []string{"A"},
				F:  []string{"B"},
				F2: []string{"C"},
				G:  floatPtr(1.1),
				H:  &t1,
				I:  &t1,
				Y:  &TestStdAux{A: "zzz"},
				Z:  &TestStdAux{},
				Z2: &TestStdAux{A: "x1"},
			},
			TestPtr{
				A:  strPtr("x"),
				B:  strPtr(""),
				C:  nil,
				D:  intPtr(0),
				E:  []string{},
				F:  []string{"F"},
				F2: nil,
				I:  &t2,
				Z:  &TestStdAux{B: "aaa"},
				Z2: nil,
			},
			TestPtr{
				A:  strPtr("x"),
				B:  strPtr("abc"),
				C:  intPtr(222),
				D:  intPtr(999),
				E:  []string{"A"},
				F:  []string{"F"},
				F2: []string{"C"},
				G:  floatPtr(1.1),
				H:  &t1,
				I:  &t2,
				Y:  &TestStdAux{A: "zzz"},
				Z:  &TestStdAux{B: "aaa"},
				Z2: &TestStdAux{A: "x1"},
			},
			TestPtr{
				A:  strPtr("x"),
				B:  strPtr(""),
				C:  intPtr(222),
				D:  intPtr(0),
				E:  []string{},
				F:  []string{"F"},
				F2: []string{},
				G:  floatPtr(1.1),
				H:  &t1,
				I:  &t2,
				Y:  &TestStdAux{A: "zzz"},
				Z:  &TestStdAux{B: "aaa"},
				Z2: &TestStdAux{A: "x1"},
			},
		},
		{
			TestPtrOmit{
				A:  strPtr(""),
				B:  strPtr("abc"),
				C:  intPtr(222),
				D:  intPtr(999),
				E:  []string{"A"},
				F:  []string{"B"},
				F2: []string{"C"},
				G:  floatPtr(1.1),
				H:  &t1,
				I:  &t1,
				Y:  &TestStdAux{A: "zzz"},
				Z:  &TestStdAux{},
			},
			TestPtrOmit{
				A:  strPtr("x"),
				B:  strPtr(""),
				C:  nil,
				D:  intPtr(0),
				E:  []string{},
				F:  []string{"F"},
				F2: nil,
				I:  &t2,
				Z:  &TestStdAux{B: "aaa"},
			},
			TestPtrOmit{
				A:  strPtr("x"),
				B:  strPtr("abc"),
				C:  intPtr(222),
				D:  intPtr(999),
				E:  []string{"A"},
				F:  []string{"F"},
				F2: []string{"C"},
				G:  floatPtr(1.1),
				H:  &t1,
				I:  &t2,
				Y:  &TestStdAux{A: "zzz"},
				Z:  &TestStdAux{B: "aaa"},
			},
			TestPtrOmit{
				A:  strPtr("x"),
				B:  strPtr(""),
				C:  intPtr(222),
				D:  intPtr(0),
				E:  []string{"A"},
				F:  []string{"F"},
				F2: []string{"C"},
				G:  floatPtr(1.1),
				H:  &t1,
				I:  &t2,
				Y:  &TestStdAux{A: "zzz"},
				Z:  &TestStdAux{B: "aaa"},
			},
		},
		{
			TestDeepMerge{Envs: map[string]string{"A": "B", "C": "D"}, A: TestStdAux{A: "a"}},
			TestDeepMerge{Envs: map[string]string{"A": "X", "Y": "Z"}, A: TestStdAux{B: "b"}},
			TestDeepMerge{Envs: map[string]string{"A": "X", "C": "D", "Y": "Z"}, A: TestStdAux{A: "a", B: "b"}},
			TestDeepMerge{Envs: map[string]string{"A": "X", "C": "D", "Y": "Z"}, A: TestStdAux{A: "", B: "b"}},
		},
	}
	for i, t := range tests {
		sc, err := FindNScopedConfig("testcoll")
		c.Assert(err, check.IsNil)
		err = sc.Save("", t.base)
		c.Assert(err, check.IsNil)
		err = sc.Save("p1", t.pool)
		c.Assert(err, check.IsNil)
		base1 := reflect.New(reflect.TypeOf(t.base))
		result1 := reflect.New(reflect.TypeOf(t.base))
		err = sc.LoadWithBase("", base1.Interface(), result1.Interface())
		c.Assert(err, check.IsNil)
		c.Assert(base1.Elem().Interface(), check.DeepEquals, t.base)
		c.Assert(result1.Elem().Interface(), check.DeepEquals, t.base)
		base2 := reflect.New(reflect.TypeOf(t.base))
		result2 := reflect.New(reflect.TypeOf(t.base))
		err = sc.LoadWithBase("p1", base2.Interface(), result2.Interface())
		c.Assert(err, check.IsNil)
		c.Assert(base2.Elem().Interface(), check.DeepEquals, t.base)
		c.Assert(result2.Elem().Interface(), check.DeepEquals, t.expected, check.Commentf("test %d", i))
		sc.AllowEmpty = true
		result3 := reflect.New(reflect.TypeOf(t.base))
		err = sc.Load("p1", result3.Interface())
		c.Assert(err, check.IsNil)
		c.Assert(result3.Elem().Interface(), check.DeepEquals, t.expectedEmpty)
		all := reflect.MakeMap(reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(t.base)))
		err = sc.LoadAll(all.Interface())
		c.Assert(err, check.IsNil)
		expected := reflect.MakeMap(reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(t.base)))
		expected.SetMapIndex(reflect.ValueOf(""), reflect.ValueOf(t.base))
		expected.SetMapIndex(reflect.ValueOf("p1"), reflect.ValueOf(t.expectedEmpty))
		c.Assert(all.Interface(), check.DeepEquals, expected.Interface())
	}
}

func (s *S) TestNScopedConfigSaveMerge(c *check.C) {
	sc, err := FindNScopedConfig("testcoll")
	c.Assert(err, check.IsNil)
	err = sc.SaveMerge("", TestDeepMerge{Envs: map[string]string{"A": "a1"}, A: TestStdAux{A: "a2"}})
	c.Assert(err, check.IsNil)
	var result1 TestDeepMerge
	err = sc.LoadBase(&result1)
	c.Assert(err, check.IsNil)
	c.Assert(result1, check.DeepEquals, TestDeepMerge{
		Envs: map[string]string{"A": "a1"},
		A:    TestStdAux{A: "a2"},
	})
	err = sc.SaveMerge("", TestDeepMerge{Envs: map[string]string{"B": "b1"}, A: TestStdAux{B: "b2"}})
	c.Assert(err, check.IsNil)
	var result2 TestDeepMerge
	err = sc.LoadBase(&result2)
	c.Assert(err, check.IsNil)
	c.Assert(result2, check.DeepEquals, TestDeepMerge{
		Envs: map[string]string{"A": "a1", "B": "b1"},
		A:    TestStdAux{A: "a2", B: "b2"},
	})
	err = sc.SaveMerge("", TestDeepMerge{Envs: map[string]string{"B": ""}, A: TestStdAux{B: ""}})
	c.Assert(err, check.IsNil)
	var result3 TestDeepMerge
	err = sc.LoadBase(&result3)
	c.Assert(err, check.IsNil)
	c.Assert(result3, check.DeepEquals, TestDeepMerge{
		Envs: map[string]string{"A": "a1"},
		A:    TestStdAux{A: "a2", B: "b2"},
	})
	sc.AllowEmpty = true
	err = sc.SaveMerge("", TestDeepMerge{Envs: map[string]string{"A": ""}, A: TestStdAux{}})
	c.Assert(err, check.IsNil)
	var result4 TestDeepMerge
	err = sc.LoadBase(&result4)
	c.Assert(err, check.IsNil)
	c.Assert(result4, check.DeepEquals, TestDeepMerge{
		Envs: map[string]string{"A": ""},
		A:    TestStdAux{A: "", B: ""},
	})
}
