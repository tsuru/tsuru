package deploy

import (
	"bytes"
	"launchpad.net/gocheck"
)

func (s *S) TestDeploy(c *gocheck.C) {
	app := &fakeApp{name: "cribcaged"}
	w := &bytes.Buffer{}
	d := GitDeployer{}
	err := d.Deploy(app, w)
	c.Assert(err, gocheck.IsNil)
	expected := make([]string, 3)
	// also ensures execution order
	expected[0] = "git clone git://tsuruhost.com/cribcaged.git test/dir --depth 1" // the command expected to run on the units
	expected[1] = "install deps"
	expected[2] = "restart"
	c.Assert(app.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestDeployLogsActions(c *gocheck.C) {
	app := &fakeApp{name: "cribcaged"}
	w := &bytes.Buffer{}
	d := GitDeployer{}
	err := d.Deploy(app, w)
	c.Assert(err, gocheck.IsNil)
	logs := w.String()
	expected := `
 ---> Tsuru receiving push

 ---> Replicating the application repository across units

 ---> Installing dependencies

 ---> Deploy done!

`
	c.Assert(logs, gocheck.Equals, expected)
}
