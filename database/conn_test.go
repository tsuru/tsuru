package database_test

import (
	. "github.com/timeredbull/tsuru/database"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestShouldExportDbSessionVariable(c *C) {
	c.Assert(Db, FitsTypeOf, &mgo.Database{})
}
