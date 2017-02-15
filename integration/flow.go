// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"gopkg.in/check.v1"
)

type hookFunc func(c *check.C, res *Result)

type CmdWithExp struct {
	C *Command
	E []Expected
}

type ExecFlow struct {
	actions  []CmdWithExp
	rollback []CmdWithExp
	hooks    map[int][]hookFunc
	provides []string
	matrix   map[string]string
}

func (f *ExecFlow) Add(cmd *Command, exp ...Expected) *ExecFlow {
	f.actions = append(f.actions, CmdWithExp{C: cmd, E: exp})
	return f
}

func (f *ExecFlow) AddRollback(cmd *Command, exp ...Expected) *ExecFlow {
	f.rollback = append(f.rollback, CmdWithExp{C: cmd, E: exp})
	return f
}

func (f *ExecFlow) AddHook(fn hookFunc) {
	if f.hooks == nil {
		f.hooks = make(map[int][]hookFunc)
	}
	pos := len(f.actions) - 1
	f.hooks[pos] = append(f.hooks[pos], fn)
}

func (f *ExecFlow) Rollback(c *check.C, env *Environment) {
	f.forExpanded(env, func() {
		f.rollbackOnce(c, env)
	})
}

func (f *ExecFlow) Run(c *check.C, env *Environment) {
	f.forExpanded(env, func() {
		f.runOnce(c, env)
	})
}

func (f *ExecFlow) rollbackOnce(c *check.C, env *Environment) {
	for i := len(f.rollback) - 1; i >= 0; i-- {
		cmd := f.rollback[i]
		res := cmd.C.Run(env)
		if len(cmd.E) == 0 {
			c.Check(res, ResultOk)
		}
		for _, exp := range cmd.E {
			c.Check(res, ResultMatches, exp)
		}
	}
}

func (f *ExecFlow) runOnce(c *check.C, env *Environment) {
	for i, cmd := range f.actions {
		res := cmd.C.Run(env)
		if len(cmd.E) == 0 {
			c.Assert(res, ResultOk)
		}
		if !env.IsDry() {
			for _, exp := range cmd.E {
				c.Assert(res, ResultMatches, exp)
			}
		}
		for _, hook := range f.hooks[i] {
			hook(c, res)
		}
	}
	for _, envVar := range f.provides {
		c.Assert(env.Has(envVar), check.Equals, true)
	}
}

func (f *ExecFlow) expandMatrix(env *Environment) []map[string]string {
	expanded := make([]map[string]string, 1)
	for k, v := range f.matrix {
		values := env.All(v)
		entries := []map[string]string{}
		for x := range expanded {
			for y := range values {
				mapValue := map[string]string{}
				if expanded[x] != nil {
					for k, v := range expanded[x] {
						mapValue[k] = v
					}
				}
				mapValue[k] = values[y]
				entries = append(entries, mapValue)
			}
		}
		expanded = entries
	}
	return expanded
}

func (f *ExecFlow) forExpanded(env *Environment, fn func()) {
	expanded := f.expandMatrix(env)
	for _, entry := range expanded {
		for k, v := range entry {
			env.Set(k, v)
		}
		fn()
	}
}
