// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"sort"

	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestDockerLogUpdate(c *check.C) {
	testCases := []struct {
		conf    provision.ScopedConfig
		entries []provision.Entry
		pools   []provision.PoolEntry
		err     error
	}{
		{
			provision.ScopedConfig{
				Envs: []provision.Entry{
					{Name: "log-driver", Value: "fluentd"},
					{Name: "fluentd-address", Value: "localhost:24224"},
				},
			}, []provision.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "fluentd-address", Value: "localhost:24224"},
			}, []provision.PoolEntry{}, nil,
		},
		{
			provision.ScopedConfig{
				Envs: []provision.Entry{
					{Name: "log-driver", Value: "bs"},
					{Name: "tag", Value: "ahoy"},
				},
			}, []provision.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "fluentd-address", Value: "localhost:24224"},
			}, []provision.PoolEntry{}, ErrLogDriverBSNoParams,
		},
		{
			provision.ScopedConfig{
				Envs: []provision.Entry{
					{Name: "tag", Value: "ahoy"},
				},
			}, []provision.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "fluentd-address", Value: "localhost:24224"},
			}, []provision.PoolEntry{}, ErrLogDriverMandatory,
		},
		{
			provision.ScopedConfig{
				Envs: []provision.Entry{
					{Name: "log-driver", Value: "bs"},
				},
			}, []provision.Entry{
				{Name: "log-driver", Value: "bs"},
			}, []provision.PoolEntry{}, nil,
		},
		{
			provision.ScopedConfig{
				Envs: []provision.Entry{
					{Name: "log-driver", Value: "fluentd"},
					{Name: "tag", Value: "x"},
				},
			}, []provision.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "tag", Value: "x"},
			}, []provision.PoolEntry{}, nil,
		},
		{
			provision.ScopedConfig{
				Pools: []provision.PoolEntry{
					{Name: "p1", Envs: []provision.Entry{
						{Name: "log-driver", Value: "journald"},
						{Name: "tag", Value: "y"},
					}},
				},
			}, []provision.Entry{
				{Name: "log-driver", Value: "fluentd"},
				{Name: "tag", Value: "x"},
			}, []provision.PoolEntry{
				{Name: "p1", Envs: []provision.Entry{
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
		conf, err := provision.FindScopedConfig(dockerLogConfigEntry)
		c.Assert(err, check.IsNil)
		sort.Sort(provision.ConfigEntryList(conf.Envs))
		sort.Sort(provision.ConfigEntryList(testData.entries))
		sort.Sort(provision.ConfigPoolEntryList(conf.Pools))
		sort.Sort(provision.ConfigPoolEntryList(testData.pools))
		for i := range conf.Pools {
			sort.Sort(provision.ConfigEntryList(conf.Pools[i].Envs))
		}
		for i := range testData.pools {
			sort.Sort(provision.ConfigEntryList(testData.pools[i].Envs))
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
