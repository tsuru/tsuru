package unit_test

import (
	"flag"
	"github.com/timeredbull/tsuru/api/unit"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

var jujuEnabled = flag.Bool("juju", false, "enable unit tests that require juju")

func (s *S) SetUpSuite(c *C) {
	if !*jujuEnabled {
		c.Skip("unit tests need juju installed (-juju to enable)")
	}
}

func (s *S) TestCreateAndDestroy(c *C) {
	u := unit.Unit{Type: "django", Name: "myUnit"}

	err := u.Create()
	c.Assert(err, IsNil)

	err = u.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestCommand(c *C) {
	u := unit.Unit{Type: "django", Name: "myUnit"}

	err := u.Create()
	c.Assert(err, IsNil)

	output, err := u.Command("uname")
	c.Assert(err, IsNil)
	c.Assert(string(output), Equals, "Linux")

	err = u.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestSendFile(c *C) {
	u := unit.Unit{Type: "django", Name: "myUnit"}

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
	appUnit := unit.Unit{Type: "django", Name: "myUnit"}
	serviceUnit := unit.Unit{Type: "mysql", Name: "MyService"}

	err := appUnit.AddRelation(&serviceUnit)
	c.Assert(err, IsNil)
}

func (s *S) TestRemoveRelation(c *C) {
	appUnit := unit.Unit{Type: "django", Name: "myUnit"}
	serviceUnit := unit.Unit{Type: "mysql", Name: "MyService"}

	err := appUnit.RemoveRelation(&serviceUnit)
	c.Assert(err, IsNil)
}
