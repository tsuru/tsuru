package gitosis

import (
	"github.com/kless/goconfig/config"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
)

func (s *S) TestAddProject(c *C) {
	err := AddProject("someProject")
	c.Assert(err, IsNil)

	conf, err := config.ReadDefault(path.Join(s.gitRoot, "gitosis-admin/gitosis.conf"))
	c.Assert(err, IsNil)
	//ensures that project have been added to gitosis.conf
	c.Assert(conf.HasSection("group someProject"), Equals, true)

	//ensures that file is not overriden when a new project is added
	err = AddProject("someOtherProject")
	c.Assert(err, IsNil)
	// it should have both sections
	conf, err = config.ReadDefault(path.Join(s.gitRoot, "gitosis-admin/gitosis.conf"))
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
