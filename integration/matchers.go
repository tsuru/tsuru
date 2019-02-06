// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"

	check "gopkg.in/check.v1"
)

type baseMatcher struct {
	name   string
	params []string
	check  func(result *Result, params []interface{}) (bool, string)
}

func (m *baseMatcher) Info() *check.CheckerInfo {
	return &check.CheckerInfo{
		Name:   m.name,
		Params: m.params,
	}
}

func (m *baseMatcher) Check(params []interface{}, names []string) (bool, string) {
	result, ok := params[0].(*Result)
	if !ok {
		return false, fmt.Sprintf("first param must be a *Result, got %T", params[0])
	}
	return m.check(result, params)
}

var ResultOk = &baseMatcher{
	name:   "ResultOK",
	params: []string{"result"},
	check: func(result *Result, params []interface{}) (bool, string) {
		if result.Error != nil || result.ExitCode != 0 {
			return false, fmt.Sprintf("result error: %v", result)
		}
		if result.Timeout {
			return false, fmt.Sprintf("result timeout after %v: %v", result.Command.Timeout, result)
		}
		return true, ""
	},
}

var ResultMatches = &baseMatcher{
	name:   "ResultMatches",
	params: []string{"result", "expected"},
	check: func(result *Result, params []interface{}) (bool, string) {
		expected, ok := params[1].(Expected)
		if !ok {
			return false, fmt.Sprintf("second param must be a Expected, got %T", params[0])
		}
		err := result.Compare(expected)
		if err != nil {
			return false, fmt.Sprintf("%v\n%v", err.Error(), result)
		}
		return true, ""
	},
}
