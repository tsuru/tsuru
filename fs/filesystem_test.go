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

func (s *S) TestOsFSCreatesTheFileInTheDisc(c *C) {
	path := "/tmp/test-fs-tsuru"
	os.Remove(path)
	defer os.Remove(path)
	fs := OsFS{}
	f, err := fs.Create(path)
	c.Assert(err, IsNil)
	defer f.Close()
	_, err = os.Stat(path)
	c.Assert(err, IsNil)
}

func (s *S) TestOsFSOpenOpensTheFileFromDisc(c *C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, IsNil)
	f.Close()
	fs := OsFS{}
	file, err := fs.Open(path)
	c.Assert(err, IsNil)
	file.Close()
	_, err = fs.Open(unknownPath)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestOsFSRemoveDeletesTheFileFromDisc(c *C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	// Remove the file even if the test fails.
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, IsNil)
	f.Close()
	fs := OsFS{}
	err = fs.Remove(path)
	c.Assert(err, IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), Equals, true)
	err = fs.Remove(unknownPath)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestOsFSRemoveAllDeletesDirectoryFromDisc(c *C) {
	path := "/tmp/tsuru/test-fs-tsuru"
	err := os.MkdirAll(path, 0755)
	c.Assert(err, IsNil)
	// Remove the directory even if the test fails.
	defer os.RemoveAll(path)
	fs := OsFS{}
	err = fs.RemoveAll(path)
	c.Assert(err, IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestOsFSStatChecksTheFileInTheDisc(c *C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, IsNil)
	f.Close()
	fs := OsFS{}
	_, err = fs.Stat(path)
	c.Assert(err, IsNil)
	_, err = fs.Stat(unknownPath)
	c.Assert(os.IsNotExist(err), Equals, true)
}
