package database_test

import (
	"testing"
	"database/sql"
	. "launchpad.net/gocheck"
	. "github.com/timeredbull/tsuru/database"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestShouldExportDbSessionVariable(c *C) {
	c.Assert(Db, FitsTypeOf, &sql.DB{})
}
