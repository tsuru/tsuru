// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package term

import (
	. "launchpad.net/gocheck"
	"os"
	"syscall"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct {
	stdin *os.File
}

var _ = Suite(&S{})

func (s *S) patchStdin(c *C, content []byte) {
	f, err := os.OpenFile("/tmp/passwdfile.txt", syscall.O_RDWR|syscall.O_NDELAY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	c.Assert(err, IsNil)
	n, err := f.Write(content)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(content))
	ret, err := f.Seek(0, 0)
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, int64(0))
	s.stdin = os.Stdin
	os.Stdin = f
}

func (s *S) unpatchStdin() {
	os.Stdin = s.stdin
}

func (s *S) TestGetPassword(c *C) {
	s.patchStdin(c, []byte("chico\n"))
	defer s.unpatchStdin()
	pass, err := ReadPassword(os.Stdin.Fd())
	c.Assert(err, IsNil)
	c.Assert(pass, Equals, "chico")
}

func (s *S) TestGetPasswordShouldRemoveAllNewLineCharactersFromTheEndOfThePassword(c *C) {
	s.patchStdin(c, []byte("chico\n\n\n"))
	defer s.unpatchStdin()
	pass, err := ReadPassword(os.Stdin.Fd())
	c.Assert(err, IsNil)
	c.Assert(pass, Equals, "chico")
}

func (s *S) TestGetPasswordShouldRemoveCarriageReturnCharacterFromTheEndOfThePassword(c *C) {
	s.patchStdin(c, []byte("opeth\r\n"))
	defer s.unpatchStdin()
	pass, err := ReadPassword(os.Stdin.Fd())
	c.Assert(err, IsNil)
	c.Assert(pass, Equals, "opeth")
}

func (s *S) TestGetPasswordWithEmptyPassword(c *C) {
	values := []string{"\n", "\r", "\r\n", ""}
	for _, value := range values {
		s.patchStdin(c, []byte(value))
		defer s.unpatchStdin()
		pass, err := ReadPassword(os.Stdin.Fd())
		c.Assert(err, IsNil)
		c.Assert(pass, Equals, "")
	}
}
