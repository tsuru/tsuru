package unit

import (
	"github.com/timeredbull/commandmocker"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
)

func (s *S) TestCreateAndDestroy(c *C) {
	u := Unit{Type: "django", Name: "myUnit"}
	err := u.Create()
	c.Assert(err, IsNil)
	err = u.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestCommand(c *C) {
	u := Unit{Type: "django", Name: "myUnit", Machine: 1}
	err := u.Create()
	c.Assert(err, IsNil)
	output, err := u.Command("uname")
	c.Assert(err, IsNil)
	c.Assert(string(output), Equals, "Linux")
	err = u.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestCommandShouldAcceptMultipleParams(c *C) {
	output := "$*"
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{Type: "django", Name: "myUnit", Machine: 1}
	err = u.Create()
	out, err := u.Command("uname", "-a")
	c.Assert(string(out), Matches, ".* uname -a")
}

func (s *S) TestSendFile(c *C) {
	u := Unit{Type: "django", Name: "myUnit"}

	err := u.Create()
	c.Assert(err, IsNil)

	file, err := ioutil.TempFile("", "upload")
	c.Assert(err, IsNil)

	defer os.Remove(file.Name())
	defer file.Close()

	err = u.SendFile(file.Name(), "/home/ubuntu")
	c.Assert(err, IsNil)

	err = u.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestAddRelation(c *C) {
	appUnit := Unit{Type: "django", Name: "myUnit"}
	serviceUnit := Unit{Type: "mysql", Name: "MyService"}

	err := appUnit.AddRelation(&serviceUnit)
	c.Assert(err, IsNil)
}

func (s *S) TestRemoveRelation(c *C) {
	appUnit := Unit{Type: "django", Name: "myUnit"}
	serviceUnit := Unit{Type: "mysql", Name: "MyService"}

	err := appUnit.RemoveRelation(&serviceUnit)
	c.Assert(err, IsNil)
}

func (s *S) TestExecuteHook(c *C) {
	appUnit := Unit{Type: "django", Name: "myUnit"}

	_, err := appUnit.ExecuteHook("requirements")
	c.Assert(err, IsNil)
}
