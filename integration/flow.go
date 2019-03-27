// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"strconv"
	"sync"

	check "gopkg.in/check.v1"
)

type hookFunc func(c *check.C, env *Environment)

type CmdWithExp struct {
	C *Command
	E []Expected
}

type ExecFlow struct {
	provides []string
	requires []string
	matrix   map[string]string
	parallel bool
	forward  hookFunc
	backward hookFunc
}

func (f *ExecFlow) Rollback(c *check.C, env *Environment) {
	if f.backward == nil {
		return
	}
	if noRollback, _ := strconv.ParseBool(env.Get("no_rollback")); noRollback {
		env.Set("dryrun", "true")
	}
	if noRollback, _ := strconv.ParseBool(env.Get("no_rollback_on_error")); c.Failed() && noRollback {
		env.Set("dryrun", "true")
	}
	f.forExpanded(env, func(e *Environment) {
		f.backward(c, e)
	})
}

func (f *ExecFlow) Run(c *check.C, env *Environment) {
	if f.forward == nil {
		return
	}
	f.forExpanded(env, func(e *Environment) {
		if c.Failed() {
			return
		}
		f.forward(c, e)
	})
	if c.Failed() {
		c.FailNow()
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

func (f *ExecFlow) forExpanded(env *Environment, fn func(env *Environment)) {
	expanded := f.expandMatrix(env)
	wg := sync.WaitGroup{}
	maxConcurrency, _ := strconv.Atoi(env.Get("maxconcurrency"))
	if maxConcurrency == 0 {
		maxConcurrency = 100
	}
	limiter := make(chan struct{}, maxConcurrency)
expandedloop:
	for _, entry := range expanded {
		newEnv := env.Clone()
		for k, v := range entry {
			newEnv.SetLocal(k, v)
		}
		for _, req := range f.requires {
			if !newEnv.Has(req) {
				continue expandedloop
			}
		}
		if f.parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				limiter <- struct{}{}
				defer func() { <-limiter }()
				fn(newEnv)
			}()
		} else {
			fn(newEnv)
		}
	}
	wg.Wait()
}
