// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import "launchpad.net/gocheck"

func (S) TestFakeGuesser(c *gocheck.C) {
	guesser := FakeGuesser{Name: "someapp"}
	name, err := guesser.GuessName("/home/user")
	c.Assert(err, gocheck.IsNil)
	c.Assert(name, gocheck.Equals, "someapp")
	c.Assert(guesser.HasGuess("/home/user"), gocheck.Equals, true)
	c.Assert(guesser.HasGuess("/usr/home/user"), gocheck.Equals, false)
}

func (S) TestFailingFakeGuesser(c *gocheck.C) {
	guesser := FailingFakeGuesser{ErrorMessage: "something went wrong"}
	name, err := guesser.GuessName("/home/user")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "something went wrong")
	c.Assert(name, gocheck.Equals, "")
	c.Assert(guesser.HasGuess("/home/user"), gocheck.Equals, true)
	c.Assert(guesser.HasGuess("/usr/home/user"), gocheck.Equals, false)
}
