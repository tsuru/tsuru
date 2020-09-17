// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scopedconfig

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	check "gopkg.in/check.v1"
)

type S struct {
	storage *db.Storage
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "scopedconfig_tests_s")
	var err error
	s.storage, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.storage.Apps().Database)
	s.storage.Close()
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.storage.Apps().Database)
	c.Assert(err, check.IsNil)
}

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

func (s *S) TestScopedConfigMulti(c *check.C) {
	t1 := time.Unix(time.Now().Unix(), 0).UTC()
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
		base                  interface{}
		pool                  interface{}
		expected              interface{}
		expectedEmpty         interface{}
		expectedPtrNilIsEmpty interface{}
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
			TestStd{
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
			TestStdOmit{
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
			TestPtr{
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
				Z2: nil,
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
				Z2: &TestStdAux{},
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
				Z2: nil,
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
				Z2: &TestStdAux{},
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
				Z2: &TestStdAux{},
			},
		},
		{
			TestDeepMerge{Envs: map[string]string{"A": "B", "C": "D"}, A: TestStdAux{A: "a"}},
			TestDeepMerge{Envs: map[string]string{"A": "X", "Y": "Z"}, A: TestStdAux{B: "b"}},
			TestDeepMerge{Envs: map[string]string{"A": "X", "C": "D", "Y": "Z"}, A: TestStdAux{A: "a", B: "b"}},
			TestDeepMerge{Envs: map[string]string{"A": "X", "C": "D", "Y": "Z"}, A: TestStdAux{A: "", B: "b"}},
			TestDeepMerge{Envs: map[string]string{"A": "X", "C": "D", "Y": "Z"}, A: TestStdAux{A: "a", B: "b"}},
		},
	}
	for i, t := range tests {
		sc := FindScopedConfig("testcoll")
		err := sc.Save("", t.base)
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
		all := reflect.New(reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(t.base)))
		err = sc.LoadAll(all.Interface())
		c.Assert(err, check.IsNil)
		expected := reflect.MakeMap(reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(t.base)))
		expected.SetMapIndex(reflect.ValueOf(""), reflect.ValueOf(t.base))
		expected.SetMapIndex(reflect.ValueOf("p1"), reflect.ValueOf(t.expectedEmpty))
		c.Assert(all.Elem().Interface(), check.DeepEquals, expected.Interface())
		sc.AllowEmpty = false
		sc.PtrNilIsEmpty = true
		result4 := reflect.New(reflect.TypeOf(t.base))
		err = sc.Load("p1", result4.Interface())
		c.Assert(err, check.IsNil)
		c.Assert(result4.Elem().Interface(), check.DeepEquals, t.expectedPtrNilIsEmpty, check.Commentf("test %d", i))
	}
}

func (s *S) TestScopedConfigSaveMerge(c *check.C) {
	sc := FindScopedConfig("testcoll")
	err := sc.SaveMerge("", TestDeepMerge{Envs: map[string]string{"A": "a1"}, A: TestStdAux{A: "a2"}})
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

func (s *S) TestScopedConfigSetFieldAtomic(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(10)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	nRoutines := 50
	values := make([]bool, nRoutines)
	var wg sync.WaitGroup
	getTokenRoutine := func(wg *sync.WaitGroup, i int) {
		defer wg.Done()
		conf := FindScopedConfig("x")
		isSet, err := conf.SetFieldAtomic("", "myvalue", fmt.Sprintf("val-%d", i))
		c.Assert(err, check.IsNil)
		values[i] = isSet
	}
	for i := 0; i < nRoutines; i++ {
		wg.Add(1)
		go getTokenRoutine(&wg, i)
	}
	wg.Wait()
	var valueSet *int
	for i := range values {
		if values[i] {
			c.Assert(valueSet, check.IsNil)
			valueSet = new(int)
			*valueSet = i
		}
	}
	c.Assert(valueSet, check.NotNil)
	conf := FindScopedConfig("x")
	var val struct{ Myvalue string }
	err := conf.LoadBase(&val)
	c.Assert(err, check.IsNil)
	c.Assert(val.Myvalue, check.Equals, fmt.Sprintf("val-%d", *valueSet))
}

func (s *S) TestScopedConfigSetField(c *check.C) {
	conf := FindScopedConfig("x")
	var val1, val2 struct{ Myvalue string }
	err := conf.SetField("", "myvalue", "v1")
	c.Assert(err, check.IsNil)
	err = conf.LoadBase(&val1)
	c.Assert(err, check.IsNil)
	c.Assert(val1.Myvalue, check.Equals, "v1")
	err = conf.SetField("", "myvalue", "v2")
	c.Assert(err, check.IsNil)
	err = conf.LoadBase(&val2)
	c.Assert(err, check.IsNil)
	c.Assert(val2.Myvalue, check.Equals, "v2")
}

func (s *S) TestScopedConfigRemove(c *check.C) {
	conf := FindScopedConfig("x")
	err := conf.Save("", TestStdAux{A: "x1", B: "y"})
	c.Assert(err, check.IsNil)
	err = conf.Save("p1", TestStdAux{A: "x2", B: "y"})
	c.Assert(err, check.IsNil)
	err = conf.Save("p2", TestStdAux{A: "x3", B: "y"})
	c.Assert(err, check.IsNil)
	var all map[string]TestStdAux
	err = conf.LoadAll(&all)
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, map[string]TestStdAux{
		"":   {A: "x1", B: "y"},
		"p1": {A: "x2", B: "y"},
		"p2": {A: "x3", B: "y"},
	})
	err = conf.Remove("p2")
	c.Assert(err, check.IsNil)
	err = conf.LoadAll(&all)
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, map[string]TestStdAux{
		"":   {A: "x1", B: "y"},
		"p1": {A: "x2", B: "y"},
	})
	err = conf.RemoveField("p1", "a")
	c.Assert(err, check.IsNil)
	err = conf.LoadAll(&all)
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, map[string]TestStdAux{
		"":   {A: "x1", B: "y"},
		"p1": {A: "x1", B: "y"},
	})
}

func (s *S) TestScopedConfigForName(c *check.C) {
	conf := FindScopedConfigFor("x", "a")
	err := conf.Save("", TestStdAux{A: "x1", B: "y"})
	c.Assert(err, check.IsNil)
	err = conf.Save("p1", TestStdAux{A: "x2", B: "y"})
	c.Assert(err, check.IsNil)
	conf = FindScopedConfigFor("x", "b")
	err = conf.Save("", TestStdAux{A: "Z1", B: "Zy"})
	c.Assert(err, check.IsNil)
	err = conf.Save("p2", TestStdAux{A: "Z2", B: "Zy"})
	c.Assert(err, check.IsNil)
	conf = FindScopedConfigFor("x", "a")
	var all map[string]TestStdAux
	err = conf.LoadAll(&all)
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, map[string]TestStdAux{
		"":   {A: "x1", B: "y"},
		"p1": {A: "x2", B: "y"},
	})
	conf = FindScopedConfigFor("x", "b")
	err = conf.LoadAll(&all)
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, map[string]TestStdAux{
		"":   {A: "Z1", B: "Zy"},
		"p2": {A: "Z2", B: "Zy"},
	})
}
