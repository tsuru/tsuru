// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	check "gopkg.in/check.v1"
)

type CustomSuite struct {
	SetUpSuiteFn    func(c *check.C)
	SetUpTestFn     func(c *check.C)
	TearDownTestFn  func(c *check.C)
	TearDownSuiteFn func(c *check.C)
}

func (s *CustomSuite) SetUpSuite(c *check.C) {
	if s.SetUpSuiteFn != nil {
		s.SetUpSuiteFn(c)
	}
}

func (s *CustomSuite) SetUpTest(c *check.C) {
	if s.SetUpTestFn != nil {
		s.SetUpTestFn(c)
	}
}

func (s *CustomSuite) TearDownSuite(c *check.C) {
	if s.TearDownSuiteFn != nil {
		s.TearDownSuiteFn(c)
	}
}

func (s *CustomSuite) TearDownTest(c *check.C) {
	if s.TearDownTestFn != nil {
		s.TearDownTestFn(c)
	}
}
