package cmd

import (
	. "launchpad.net/gocheck"
	"os"
	"syscall"
)

func patchStdin(c *C, content []byte) {
	f, err := os.OpenFile("/tmp/passwdfile.txt", syscall.O_RDWR|syscall.O_NDELAY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	c.Assert(err, IsNil)
	n, err := f.Write(content)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(content))
	ret, err := f.Seek(0, 0)
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, int64(0))
	os.Stdin = f
}

func unpathStdin() {
	os.Stdin = os.NewFile(uintptr(syscall.Stdin), "/dev/stdin")
}

func (s *S) TestGetPassword(c *C) {
	patchStdin(c, []byte("chico\n"))
	defer unpathStdin()
	pass := getPassword(os.Stdin.Fd())
	c.Assert(pass, Equals, "chico")
}

func (s *S) TestGetPasswordShouldRemoveAllNewLineCharactersFromTheEndOfThePassword(c *C) {
	patchStdin(c, []byte("chico\n\n\n"))
	defer unpathStdin()
	pass := getPassword(os.Stdin.Fd())
	c.Assert(pass, Equals, "chico")
}

func (s *S) TestGetPasswordShouldRemoveCarriageReturnCharacterFromTheEndOfThePassword(c *C) {
	patchStdin(c, []byte("opeth\r\n"))
	defer unpathStdin()
	pass := getPassword(os.Stdin.Fd())
	c.Assert(pass, Equals, "opeth")
}

func (s *S) TestGetPasswordWithEmptyPassword(c *C) {
	patchStdin(c, []byte("\n"))
	defer unpathStdin()
	pass := getPassword(os.Stdin.Fd())
	c.Assert(pass, Equals, "")
}
