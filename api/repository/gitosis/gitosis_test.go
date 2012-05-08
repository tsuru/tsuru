package gitosis

import (
	. "launchpad.net/gocheck"
	"os"
	"path"
	/* "github.com/timeredbull/tsuru/config" */
	"github.com/kless/goconfig/config"
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
}
