package deploy

import (
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("git:unit-repo", "test/dir")
	config.Set("git:ro-host", "tsuruhost.com")
}

func (s *S) TearDownSuite(c *gocheck.C) {
	config.Unset("git:unit-repo")
	config.Unset("git:host")
}
