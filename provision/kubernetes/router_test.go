// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	check "gopkg.in/check.v1"
)

func (s *S) Test_RoutableProvisioner_EnsureRouter(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	err := s.p.EnsureRouter(a, RouterTypeLoadbalancer, map[string]string{})
	c.Assert(err, check.IsNil)
}
