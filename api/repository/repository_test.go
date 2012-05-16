package repository_test

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/repository"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestCloneRepository(c *C) {
	a := app.App{Name: "barfoo"}
	err := repository.CloneRepository(a.Name)
	c.Assert(err, IsNil)
}

func (s *S) TestGetRepositoryUrl(c *C) {
	a := app.App{Name: "foobar"}
	url := repository.GetRepositoryUrl(a.Name)
	expected := fmt.Sprintf("git@tsuru.plataformas.glb.com:%s.git", a.Name)
	c.Assert(url, Equals, expected)
}
