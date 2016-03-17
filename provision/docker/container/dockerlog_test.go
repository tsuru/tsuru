// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"sort"

	"github.com/tsuru/tsuru/scopedconfig"
	"gopkg.in/check.v1"
)

func (s *S) TestDockerLogUpdate(c *check.C) {
	testCases := []struct {
		conf    scopedconfig.ScopedConfig
		entries []scopedconfig.Entry
		pools   []scopedconfig.PoolEntry
		err     error
	}{
		{
			scopedconfig.ScopedConfig{
				Envs: []scopedconfig.Entry{
					{Name: "log-driver", Value: "fluentd"},
					{Name: "fluentd-address", Value: "localhost:24224"},
				},
			}, []scopedconfig.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "fluentd-address", Value: "localhost:24224"},
			}, []scopedconfig.PoolEntry{}, nil,
		},
		{
			scopedconfig.ScopedConfig{
				Envs: []scopedconfig.Entry{
					{Name: "log-driver", Value: "bs"},
					{Name: "tag", Value: "ahoy"},
				},
			}, []scopedconfig.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "fluentd-address", Value: "localhost:24224"},
			}, []scopedconfig.PoolEntry{}, ErrLogDriverBSNoParams,
		},
		{
			scopedconfig.ScopedConfig{
				Envs: []scopedconfig.Entry{
					{Name: "tag", Value: "ahoy"},
				},
			}, []scopedconfig.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "fluentd-address", Value: "localhost:24224"},
			}, []scopedconfig.PoolEntry{}, ErrLogDriverMandatory,
		},
		{
			scopedconfig.ScopedConfig{
				Envs: []scopedconfig.Entry{
					{Name: "log-driver", Value: "bs"},
				},
			}, []scopedconfig.Entry{
				{Name: "log-driver", Value: "bs"},
			}, []scopedconfig.PoolEntry{}, nil,
		},
		{
			scopedconfig.ScopedConfig{
				Envs: []scopedconfig.Entry{
					{Name: "log-driver", Value: "fluentd"},
					{Name: "tag", Value: "x"},
				},
			}, []scopedconfig.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "tag", Value: "x"},
			}, []scopedconfig.PoolEntry{}, nil,
		},
		{
			scopedconfig.ScopedConfig{
				Pools: []scopedconfig.PoolEntry{
					{Name: "p1", Envs: []scopedconfig.Entry{
						{Name: "log-driver", Value: "journald"},
						{Name: "tag", Value: "y"},
					}},
				},
			}, []scopedconfig.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "tag", Value: "x"},
			}, []scopedconfig.PoolEntry{
				{Name: "p1", Envs: []scopedconfig.Entry{
					{Name: "log-driver", Value: "journald"},
					{Name: "tag", Value: "y"},
				}},
			}, nil,
		},
	}
	logConf := DockerLog{}
	for _, testData := range testCases {
		err := logConf.Update(&testData.conf)
		c.Assert(err, check.DeepEquals, testData.err)
		conf, err := scopedconfig.FindScopedConfig(dockerLogConfigEntry)
		c.Assert(err, check.IsNil)
		sort.Sort(scopedconfig.ConfigEntryList(conf.Envs))
		sort.Sort(scopedconfig.ConfigEntryList(testData.entries))
		sort.Sort(scopedconfig.ConfigPoolEntryList(conf.Pools))
		sort.Sort(scopedconfig.ConfigPoolEntryList(testData.pools))
		for i := range conf.Pools {
			sort.Sort(scopedconfig.ConfigEntryList(conf.Pools[i].Envs))
		}
		for i := range testData.pools {
			sort.Sort(scopedconfig.ConfigEntryList(testData.pools[i].Envs))
		}
		c.Assert(conf.Envs, check.DeepEquals, testData.entries)
		c.Assert(conf.Pools, check.DeepEquals, testData.pools)
	}
	driver, opts, err := logConf.LogOpts("p1")
	c.Assert(err, check.IsNil)
	c.Assert(driver, check.Equals, "journald")
	c.Assert(opts, check.DeepEquals, map[string]string{"tag": "y"})
	driver, opts, err = logConf.LogOpts("other")
	c.Assert(err, check.IsNil)
	c.Assert(driver, check.Equals, "fluentd")
	c.Assert(opts, check.DeepEquals, map[string]string{"tag": "x"})
}
