package app_test

import (
	"github.com/timeredbull/tsuru/api/app"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t)}

type S struct{}
var _ = Suite(&S{})

func (s *S) TestCreate(c *C) {
	app := app.App{}
	app.Name = "appName"
	app.Framework = "django"

	err := app.Create()
	c.Assert(err, IsNil)

	c.Assert(app.State, Equals, "Pending")
}
