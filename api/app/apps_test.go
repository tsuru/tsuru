package apps_test

import (
	"github.com/timeredbull/tsuru/api/apps"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t)}

type S struct{}
var _ = Suite(&S{})

func (s *S) TestCreate(c *C) {
	app := apps.App{}
	app.Name = "appName"
	app.Framework = "django"

	err := app.Create()
	c.Assert(err, IsNil)

	c.Assert(app.State, Equals, "Pending")
}
