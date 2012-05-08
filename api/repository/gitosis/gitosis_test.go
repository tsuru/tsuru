package gitosis

import (
	"github.com/kless/goconfig/config"
	. "launchpad.net/gocheck"
	"os"
	"path"
)

func (s *S) TestAddProject(c *C) {
	pwd := os.Getenv("PWD")
	err := os.Chdir(path.Join(s.gitRoot, "gitosis-admin"))
	_, err = os.Create("gitosis.conf")
	c.Assert(err, IsNil)
	err = os.Chdir(pwd)

	err = AddProject("someProject")
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
	pwd := os.Getenv("PWD")
	err := os.Chdir(path.Join(s.gitRoot, "gitosis-admin"))
	_, err = os.Create("gitosis.conf")
	c.Assert(err, IsNil)
	err = os.Chdir(pwd)

	err = AddProject("aProject")
	c.Assert(err, IsNil)

	err = AddProject("aProject")
	c.Assert(err, NotNil)
}
