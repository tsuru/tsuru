package fs

import (
	. "launchpad.net/gocheck"
	"os"
	"testing"
)

type S struct{}

var _ = Suite(&S{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *S) TestOsFsImplementsFS(c *C) {
	var fs Fs
	var ofs OsFs
	c.Assert(ofs, Implements, &fs)
}

func (s *S) TestOsFsCreatesTheFileInTheDisc(c *C) {
	path := "/tmp/test-fs-tsuru"
	os.Remove(path)
	defer os.Remove(path)
	fs := OsFs{}
	f, err := fs.Create(path)
	c.Assert(err, IsNil)
	defer f.Close()
	_, err = os.Stat(path)
	c.Assert(err, IsNil)
}

func (s *S) TestOsFsOpenOpensTheFileFromDisc(c *C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, IsNil)
	f.Close()
	fs := OsFs{}
	file, err := fs.Open(path)
	c.Assert(err, IsNil)
	file.Close()
	_, err = fs.Open(unknownPath)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestOsFsRemoveDeletesTheFileFromDisc(c *C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	// Remove the file even if the test fails.
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, IsNil)
	f.Close()
	fs := OsFs{}
	err = fs.Remove(path)
	c.Assert(err, IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), Equals, true)
	err = fs.Remove(unknownPath)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestOsFsRemoveAllDeletesDirectoryFromDisc(c *C) {
	path := "/tmp/tsuru/test-fs-tsuru"
	err := os.MkdirAll(path, 0755)
	c.Assert(err, IsNil)
	// Remove the directory even if the test fails.
	defer os.RemoveAll(path)
	fs := OsFs{}
	err = fs.RemoveAll(path)
	c.Assert(err, IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestOsFsStatChecksTheFileInTheDisc(c *C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, IsNil)
	f.Close()
	fs := OsFs{}
	_, err = fs.Stat(path)
	c.Assert(err, IsNil)
	_, err = fs.Stat(unknownPath)
	c.Assert(os.IsNotExist(err), Equals, true)
}
