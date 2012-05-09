package gitosis

import (
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
)

func (s *S) TestAddProject(c *C) {
	err := AddProject("someProject")
	c.Assert(err, IsNil)

	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	//ensures that project have been added to gitosis.conf
	c.Assert(conf.HasSection("group someProject"), Equals, true)

	//ensures that file is not overriden when a new project is added
	err = AddProject("someOtherProject")
	c.Assert(err, IsNil)
	// it should have both sections
	conf, err = ini.ReadDefault(path.Join(s.gitRoot, "gitosis-admin/gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someProject"), Equals, true)
	c.Assert(conf.HasSection("group someOtherProject"), Equals, true)
}

func (s *S) TestAddProjectShouldReturnErrorWhenSectionAlreadyExists(c *C) {
	err := AddProject("aProject")
	c.Assert(err, IsNil)

	err = AddProject("aProject")
	c.Assert(err, NotNil)
}

func (s *S) TestAddProjectShouldCommitAndPushChangesToGitosisBare(c *C) {
	err := AddProject("gandalf")
	c.Assert(err, IsNil)
	pwd := os.Getenv("PWD")
	os.Chdir(s.gitosisBare)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%H").CombinedOutput()
	c.Assert(err, IsNil)

	os.Chdir(s.gitosisRepo)
	repoOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%H").CombinedOutput()
	c.Assert(err, IsNil)

	os.Chdir(pwd)

	c.Assert(string(repoOutput), Equals, string(bareOutput))
}

func (s *S) TestAddMemberToProject(c *C) {
	err := AddProject("take-over-the-world") // test also with a inexistent project
	c.Assert(err, IsNil)
	err = AddMember("take-over-the-world", "brain")

	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group take-over-the-world"), Equals, true)
	c.Assert(conf.HasOption("group take-over-the-world", "member"), Equals, true)
	members, err := conf.String("group take-over-the-world", "member")
	c.Assert(err, IsNil)
	c.Assert(members, Equals, "brain")
}

func (s *S) TestAddAndCommit(c *C) {
	confPath := path.Join(s.gitosisRepo, "gitosis.conf")
	conf, err := ini.ReadDefault(confPath)
	c.Assert(err, IsNil)
	conf.AddSection("foo bar")
	PushToGitosis("Some commit message")

	pwd := os.Getenv("PWD")
	os.Chdir(s.gitosisBare)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%s").CombinedOutput()
	c.Assert(err, IsNil)
	os.Chdir(pwd)

	c.Assert(string(bareOutput), Equals, "Some commit message")
}

func (s *S) TestConfPathReturnsGitosisConfPath(c *C) {
	repoPath, err := config.GetString("git:gitosis-repo")
	expected := path.Join(repoPath, "gitosis.conf")
	obtained, err := ConfPath()
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, expected)
}
