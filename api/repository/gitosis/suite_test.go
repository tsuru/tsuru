package gitosis

import (
	"github.com/timeredbull/tsuru/config"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	gitRoot     string
	gitosisBare string
	gitosisRepo string
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	err := config.ReadConfigFile("../../../etc/tsuru.conf")
	c.Assert(err, IsNil)
	s.gitRoot, err = config.GetString("git:root")
	c.Assert(err, IsNil)
	s.gitosisBare, err = config.GetString("git:gitosis-bare")
	c.Assert(err, IsNil)
	s.gitosisRepo, err = config.GetString("git:gitosis-repo")
	currentDir := os.Getenv("PWD")
	err = os.RemoveAll(s.gitRoot)
	c.Assert(err, IsNil)
	err = os.MkdirAll(s.gitRoot, 0777)
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

func (s *S) SetUpTest(c *C) {
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	defer os.Chdir(pwd)
	err = os.Chdir(path.Join(s.gitRoot, "gitosis-admin"))
	_, err = os.Create("gitosis.conf")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	err := os.RemoveAll(s.gitRoot)
	c.Assert(err, IsNil)
}

func (s *S) TearDownTest(c *C) {
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	defer os.Chdir(pwd)
	err = os.Chdir(path.Join(s.gitRoot, "gitosis-admin"))
	err = exec.Command("git", "rm", "gitosis.conf").Run()
	if err == nil {
		err = pushToGitosis("removing test file")
		c.Assert(err, IsNil)
	}
}

func (s *S) lastBareCommit(c *C) string {
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	defer os.Chdir(pwd)
	os.Chdir(s.gitosisBare)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%s").CombinedOutput()
	c.Assert(err, IsNil)
	return string(bareOutput)
}
