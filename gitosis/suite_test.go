package gitosis

import (
	"github.com/timeredbull/tsuru/config"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	gitRoot     string
	gitosisBare string
	gitosisRepo string
}

var _ = Suite(&S{})

type isInGitosisChecker struct{}

func (c *isInGitosisChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "IsInGitosis", Params: []string{"str"}}
}

func (c *isInGitosisChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 1 {
		return false, "you should provide one string parameter"
	}
	str, ok := params[0].(string)
	if !ok {
		return false, "the parameter should be a string"
	}
	gitosisRepo, err := config.GetString("git:gitosis-repo")
	if err != nil {
		return false, "failed to get config"
	}
	path := path.Join(gitosisRepo, "gitosis.conf")
	f, err := os.Open(path)
	if err != nil {
		return false, err.Error()
	}
	defer f.Close()
	content, err := ioutil.ReadAll(f)
	if err != nil {
		return false, err.Error()
	}
	return strings.Contains(string(content), str), ""
}

var IsInGitosis, NotInGitosis Checker = &isInGitosisChecker{}, Not(IsInGitosis)

func (s *S) SetUpSuite(c *C) {
	err := config.ReadConfigFile("../etc/tsuru.conf")
	c.Assert(err, IsNil)
	s.gitRoot, err = config.GetString("git:root")
	c.Assert(err, IsNil)
	s.gitosisBare, err = config.GetString("git:gitosis-bare")
	c.Assert(err, IsNil)
	s.gitosisRepo, err = config.GetString("git:gitosis-repo")
	err = os.RemoveAll(s.gitRoot)
	c.Assert(err, IsNil)
	err = os.MkdirAll(s.gitRoot, 0777)
	c.Assert(err, IsNil)
	err = exec.Command("git", "init", "--bare", s.gitosisBare).Run()
	c.Assert(err, IsNil)
	err = exec.Command("git", "clone", s.gitosisBare, s.gitosisRepo).Run()
	c.Assert(err, IsNil)
}

func (s *S) SetUpTest(c *C) {
	fpath := path.Join(s.gitosisRepo, "gitosis.conf")
	f, err := os.Create(fpath)
	c.Assert(err, IsNil)
	f.Close()
}

func (s *S) TearDownSuite(c *C) {
	err := os.RemoveAll(s.gitRoot)
	c.Assert(err, IsNil)
}

func (s *S) TearDownTest(c *C) {
	_, err := runGit("rm", "gitosis.conf")
	if err == nil {
		err = pushToGitosis("removing test file")
		c.Assert(err, IsNil)
	}
}

func (s *S) lastBareCommit(c *C) string {
	bareOutput, err := exec.Command("git", "--git-dir="+s.gitosisBare, "log", "-1", "--pretty=format:%s").CombinedOutput()
	c.Assert(err, IsNil)
	return string(bareOutput)
}
