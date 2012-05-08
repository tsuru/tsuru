package gitosis

import (
	"github.com/timeredbull/tsuru/config"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	gitRoot string
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	err := config.ReadConfigFile("/etc/tsuru/tsuru.conf")
	c.Assert(err, IsNil)
	s.gitRoot, err = config.GetString("git:root")
	c.Assert(err, IsNil)
	currentDir := os.Getenv("PWD")
	err = os.Mkdir(s.gitRoot, 0777)
	c.Assert(err, IsNil)
	err = os.Chdir(s.gitRoot)
	c.Assert(err, IsNil)
	err = exec.Command("ls").Run()
	c.Assert(err, IsNil)
	err = exec.Command("git", "init", "--bare", "gitosis-admin.git").Run()
	c.Assert(err, IsNil)
	err = exec.Command("git", "clone", "gitosis-admin.git").Run()
	c.Assert(err, IsNil)
	err = os.Chdir(currentDir)
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	err := os.RemoveAll(s.gitRoot)
	c.Assert(err, IsNil)
}
