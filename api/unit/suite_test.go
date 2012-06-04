package unit

import (
	. "launchpad.net/gocheck"
	"os"
	"path"
	"strings"
	"syscall"
	"testing"
	"text/template"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	err := putJujuInPath("Linux")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	err := removeJujuFromPath()
	c.Assert(err, IsNil)
}

var dirname = path.Join(os.TempDir(), "juju-tsuru-unit-dir-test")

// putJujuInPath creates a juju executable in a temporary directory and adds
// this directory to the user's path. This action should be undone using
// removeJujuFromPath function.
func putJujuInPath(output string) (err error) {
	err = os.MkdirAll(dirname, 0777)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path.Join(dirname, "juju"), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0755)
	if err != nil {
		return
	}
	defer f.Close()
	t, err := template.ParseFiles("../../testdata/jujutemplate")
	if err != nil {
		return
	}
	err = t.Execute(f, output)
	if err != nil {
		return
	}
	path := os.Getenv("PATH")
	path = dirname + ":" + path
	return os.Setenv("PATH", path)
}

// removeJujuFromPath removes the temporary directory created with
// putJujuInPath from the user's path and deletes it.
func removeJujuFromPath() (err error) {
	path := os.Getenv("PATH")
	if strings.HasPrefix(path, dirname) {
		path = path[len(dirname)+1:]
		err = os.Setenv("PATH", path)
		if err != nil {
			return
		}
	}
	return os.RemoveAll(dirname)
}
