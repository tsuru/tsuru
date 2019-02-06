// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fix

import (
	"testing"

	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestGetImageDigest(c *check.C) {
	output := `
Pull output...
Digest: sha256:dockershouldhaveaeasywaytogetitfromimage
More pull output..
`
	digest, err := GetImageDigest(output)
	c.Assert(err, check.IsNil)
	c.Assert(digest, check.Equals, "sha256:dockershouldhaveaeasywaytogetitfromimage")
}

func (s *S) TestGetImageDigestNoDigest(c *check.C) {
	output := `
Pull output...
No digest here
More pull output..
`
	_, err := GetImageDigest(output)
	c.Assert(err, check.NotNil)
}
