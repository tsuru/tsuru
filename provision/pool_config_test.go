// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"fmt"
	"runtime"
	"sort"
	"sync"

	"gopkg.in/check.v1"
)

func (s *S) TestFindScopedConfig(c *check.C) {
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	c.Assert(conf.Scope, check.Equals, "x")
	conf.Add("a", "b")
	err = conf.SaveEnvs()
	c.Assert(err, check.IsNil)
	conf, err = FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	c.Assert(conf.Envs, check.DeepEquals, []Entry{{Name: "a", Value: "b"}})
}

func (s *S) TestScopedConfigAdd(c *check.C) {
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	conf.Add("a", "b")
	expected := []Entry{{Name: "a", Value: "b"}}
	c.Assert(conf.Envs, check.DeepEquals, expected)
	err = conf.SaveEnvs()
	c.Assert(err, check.IsNil)
	conf, err = FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	c.Assert(conf.Envs, check.DeepEquals, expected)
}

func (s *S) TestScopedConfigAddPool(c *check.C) {
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	conf.AddPool("p", "a", "b")
	expected := []PoolEntry{{Name: "p", Envs: []Entry{{Name: "a", Value: "b"}}}}
	c.Assert(conf.Envs, check.IsNil)
	c.Assert(conf.Pools, check.DeepEquals, expected)
	err = conf.SaveEnvs()
	c.Assert(err, check.IsNil)
	conf, err = FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	c.Assert(conf.Envs, check.DeepEquals, []Entry{})
	c.Assert(conf.Pools, check.DeepEquals, expected)
}

func (s *S) TestScopedConfigUpdateWith(c *check.C) {
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	conf.Add("a", "overriden")
	conf.Add("c", "removed")
	conf.Add("d", "dval")
	err = conf.SaveEnvs()
	c.Assert(err, check.IsNil)
	base := ScopedConfig{
		Scope: "ignored",
		Envs:  []Entry{{Name: "a", Value: "b"}, {Name: "b", Value: "c0"}, {Name: "c", Value: ""}},
		Pools: []PoolEntry{{Name: "p", Envs: []Entry{{Name: "b", Value: "c1"}}}},
		Extra: map[string]interface{}{"notcopied": "val"},
	}
	err = conf.UpdateWith(&base)
	c.Assert(err, check.IsNil)
	c.Assert(conf.Scope, check.Equals, "x")
	expectedEntries := []Entry{
		{Name: "a", Value: "b"},
		{Name: "b", Value: "c0"},
		{Name: "d", Value: "dval"},
	}
	expectedPoolEntries := []PoolEntry{{Name: "p", Envs: []Entry{{Name: "b", Value: "c1"}}}}
	sort.Sort(ConfigEntryList(expectedEntries))
	sort.Sort(ConfigPoolEntryList(expectedPoolEntries))
	sort.Sort(ConfigEntryList(conf.Envs))
	sort.Sort(ConfigPoolEntryList(conf.Pools))
	c.Assert(conf.Envs, check.DeepEquals, expectedEntries)
	c.Assert(conf.Pools, check.DeepEquals, expectedPoolEntries)
	c.Assert(conf.Extra, check.IsNil)
	dbConf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	sort.Sort(ConfigEntryList(dbConf.Envs))
	sort.Sort(ConfigPoolEntryList(dbConf.Pools))
	c.Assert(dbConf.Envs, check.DeepEquals, expectedEntries)
	c.Assert(dbConf.Pools, check.DeepEquals, expectedPoolEntries)
	c.Assert(dbConf.Extra, check.IsNil)
}

func (s *S) TestScopedConfigSetGetExtra(c *check.C) {
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	err = conf.SetExtra("extra", "val")
	c.Assert(err, check.IsNil)
	val := conf.GetExtraString("extra")
	c.Assert(val, check.Equals, "val")
	invalidVal := conf.GetExtraString("extrasomething")
	c.Assert(invalidVal, check.Equals, "")
	dbConf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	val = dbConf.GetExtraString("extra")
	c.Assert(val, check.Equals, "val")
	invalidVal = dbConf.GetExtraString("extrasomething")
	c.Assert(invalidVal, check.Equals, "")
}

func (s *S) TestScopedConfigSetExtraAtomic(c *check.C) {
	runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	nRoutines := 50
	values := make([]bool, nRoutines)
	var wg sync.WaitGroup
	getTokenRoutine := func(wg *sync.WaitGroup, i int) {
		defer wg.Done()
		conf, err := FindScopedConfig("x")
		c.Assert(err, check.IsNil)
		isSet, err := conf.SetExtraAtomic("myvalue", fmt.Sprintf("val-%d", i))
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
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	val := conf.GetExtraString("myvalue")
	c.Assert(val, check.Equals, fmt.Sprintf("val-%d", *valueSet))
}

func (s *S) TestScopedConfigPoolEntries(c *check.C) {
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	conf.Add("a", "a0")
	conf.Add("b", "b0")
	conf.AddPool("p1", "a", "a1")
	conf.AddPool("p2", "b", "b1")
	entries := conf.PoolEntries("p1")
	c.Assert(entries, check.DeepEquals, EntryMap{
		"a": Entry{Name: "a", Value: "a1"},
		"b": Entry{Name: "b", Value: "b0"},
	})
	entries = conf.PoolEntries("p2")
	c.Assert(entries, check.DeepEquals, EntryMap{
		"a": Entry{Name: "a", Value: "a0"},
		"b": Entry{Name: "b", Value: "b1"},
	})
	entries = conf.PoolEntries("p3")
	c.Assert(entries, check.DeepEquals, EntryMap{
		"a": Entry{Name: "a", Value: "a0"},
		"b": Entry{Name: "b", Value: "b0"},
	})
	err = conf.SaveEnvs()
	c.Assert(err, check.IsNil)
	dbConf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	entries = dbConf.PoolEntries("p1")
	c.Assert(entries, check.DeepEquals, EntryMap{
		"a": Entry{Name: "a", Value: "a1"},
		"b": Entry{Name: "b", Value: "b0"},
	})
}

func (s *S) TestScopedConfigPoolEntry(c *check.C) {
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	conf.Add("a", "a0")
	conf.Add("b", "b0")
	conf.AddPool("p1", "a", "a1")
	conf.AddPool("p2", "b", "b1")
	entry := conf.PoolEntry("p1", "a")
	c.Assert(entry, check.Equals, "a1")
	entry = conf.PoolEntry("p1", "b")
	c.Assert(entry, check.Equals, "b0")
	entry = conf.PoolEntry("p2", "a")
	c.Assert(entry, check.Equals, "a0")
	entry = conf.PoolEntry("p2", "b")
	c.Assert(entry, check.Equals, "b1")
	entry = conf.PoolEntry("p3", "a")
	c.Assert(entry, check.Equals, "a0")
	entry = conf.PoolEntry("p3", "b")
	c.Assert(entry, check.Equals, "b0")
	err = conf.SaveEnvs()
	c.Assert(err, check.IsNil)
	dbConf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	entry = dbConf.PoolEntry("p1", "a")
	c.Assert(entry, check.Equals, "a1")
}

func (s *S) TestScopedConfigResetEnvs(c *check.C) {
	conf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	c.Assert(conf.Envs, check.IsNil)
	conf.Add("a", "a0")
	conf.ResetEnvs()
	c.Assert(conf.Envs, check.IsNil)
	conf.Add("b", "b0")
	conf.SaveEnvs()
	dbConf, err := FindScopedConfig("x")
	c.Assert(err, check.IsNil)
	c.Assert(dbConf.Envs, check.DeepEquals, []Entry{{Name: "b", Value: "b0"}})
}
