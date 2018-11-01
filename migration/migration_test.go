// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"bytes"
	"errors"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&Suite{})

type Suite struct {
	conn *db.Storage
}

func (s *Suite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:name", "tsurud_migration_tests")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *Suite) SetUpTest(c *check.C) {
	migrations = nil
}

func (s *Suite) TearDownTest(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *Suite) TearDownSuite(c *check.C) {
	s.conn.Apps().Database.DropDatabase()
}

func (s *Suite) TestRun(c *check.C) {
	expected := `Running "migration1"... OK
Running "migration2"... OK
Running "migration3"... OK
`
	var buf bytes.Buffer
	var runs []string
	var mFunc = func(name string) MigrateFunc {
		return func() error {
			runs = append(runs, name)
			return nil
		}
	}
	err := Register("migration1", mFunc("migration1"))
	c.Assert(err, check.IsNil)
	err = Register("migration2", mFunc("migration2"))
	c.Assert(err, check.IsNil)
	err = Register("migration3", mFunc("migration3"))
	c.Assert(err, check.IsNil)
	err = RegisterOptional("migration4", mFunc("migration4"))
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Writer: &buf, Dry: false})
	c.Assert(err, check.IsNil)
	c.Assert(runs, check.DeepEquals, []string{"migration1", "migration2", "migration3"})
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *Suite) TestMultipleRuns(c *check.C) {
	var buf bytes.Buffer
	var runs []string
	var mFunc = func(name string) MigrateFunc {
		return func() error {
			runs = append(runs, name)
			return nil
		}
	}
	err := Register("migration1", mFunc("migration1"))
	c.Assert(err, check.IsNil)
	err = Register("migration2", mFunc("migration2"))
	c.Assert(err, check.IsNil)
	err = Register("migration3", mFunc("migration3"))
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Writer: &buf, Dry: false})
	c.Assert(err, check.IsNil)
	migrations = nil
	err = Register("migration1", mFunc("migration1"))
	c.Assert(err, check.IsNil)
	err = Register("migration4", mFunc("migration4"))
	c.Assert(err, check.IsNil)
	err = Register("migration2", mFunc("migration2"))
	c.Assert(err, check.IsNil)
	err = Register("migration3", mFunc("migration3"))
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Writer: &buf, Dry: false})
	c.Assert(err, check.IsNil)
	c.Assert(runs, check.DeepEquals, []string{"migration1", "migration2", "migration3", "migration4"})
}

func (s *Suite) TestFailingMigration(c *check.C) {
	var runs []string
	var calls int
	var buf bytes.Buffer
	err := Register("mig1", func() error {
		if calls == 1 {
			runs = append(runs, "mig1")
			return nil
		}
		calls++
		return errors.New("something went wrong")
	})
	c.Assert(err, check.IsNil)
	err = Register("mig2", func() error {
		runs = append(runs, "mig2")
		return nil
	})
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Writer: &buf, Dry: false})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "something went wrong")
	c.Assert(runs, check.HasLen, 0)
	err = Run(RunArgs{Writer: &buf, Dry: false})
	c.Assert(err, check.IsNil)
	c.Assert(runs, check.DeepEquals, []string{"mig1", "mig2"})
}

func (s *Suite) TestRunDryMode(c *check.C) {
	expected := `Running "migration1"... OK
Running "migration2"... OK
Running "migration3"... OK
`
	var buf bytes.Buffer
	var runs []string
	var mFunc = func(name string) MigrateFunc {
		return func() error {
			runs = append(runs, name)
			return nil
		}
	}
	err := Register("migration1", mFunc("migration1"))
	c.Assert(err, check.IsNil)
	err = Register("migration2", mFunc("migration2"))
	c.Assert(err, check.IsNil)
	err = Register("migration3", mFunc("migration3"))
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Writer: &buf, Dry: true})
	c.Assert(err, check.IsNil)
	c.Assert(runs, check.HasLen, 0)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *Suite) TestRegisterDuplicate(c *check.C) {
	err := Register("migration1", nil)
	c.Assert(err, check.IsNil)
	err = Register("migration1", nil)
	c.Assert(err, check.Equals, ErrDuplicateMigration)
}

func (s *Suite) TestRunOptional(c *check.C) {
	expected := `Running "migration2"... OK
`
	var buf bytes.Buffer
	var runs []string
	var mFunc = func(name string) MigrateFunc {
		return func() error {
			runs = append(runs, name)
			return nil
		}
	}
	err := Register("migration1", mFunc("migration1"))
	c.Assert(err, check.IsNil)
	err = RegisterOptional("migration2", mFunc("migration2"))
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Name: "migration2", Writer: &buf, Dry: false, Force: false})
	c.Assert(err, check.IsNil)
	c.Assert(runs, check.DeepEquals, []string{"migration2"})
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *Suite) TestRunOptionalAlreadyExecuted(c *check.C) {
	expected := `Running "migration2"... OK
Running "migration2"... OK
`
	var buf bytes.Buffer
	var runs []string
	var mFunc = func(name string) MigrateFunc {
		return func() error {
			runs = append(runs, name)
			return nil
		}
	}
	err := Register("migration1", mFunc("migration1"))
	c.Assert(err, check.IsNil)
	err = RegisterOptional("migration2", mFunc("migration2"))
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Name: "migration2", Writer: &buf, Dry: false, Force: false})
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Name: "migration2", Writer: &buf, Dry: false, Force: false})
	c.Assert(err, check.Equals, ErrMigrationAlreadyExecuted)
	err = Run(RunArgs{Name: "migration2", Writer: &buf, Dry: false, Force: true})
	c.Assert(err, check.IsNil)
	c.Assert(runs, check.DeepEquals, []string{"migration2", "migration2"})
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *Suite) TestRunOptionalNotFound(c *check.C) {
	var buf bytes.Buffer
	err := Register("migration1", func() error { return nil })
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Name: "migration1", Writer: &buf, Dry: false, Force: false})
	c.Assert(err, check.Equals, ErrMigrationMandatory)
	err = Run(RunArgs{Name: "migration2", Writer: &buf, Dry: false, Force: false})
	c.Assert(err, check.Equals, ErrMigrationNotFound)
}

func (s *Suite) TestList(c *check.C) {
	var buf bytes.Buffer
	nilFn := func() error { return nil }
	err := Register("migration1", nilFn)
	c.Assert(err, check.IsNil)
	err = RegisterOptional("migration2", nilFn)
	c.Assert(err, check.IsNil)
	err = RegisterOptional("migration3", nilFn)
	c.Assert(err, check.IsNil)
	err = Run(RunArgs{Name: "migration3", Writer: &buf})
	c.Assert(err, check.IsNil)
	migrationsList, err := List()
	c.Assert(err, check.IsNil)
	for i := range migrationsList {
		migrationsList[i].fn = nil
	}
	c.Assert(migrationsList, check.DeepEquals, []migration{
		{Name: "migration1"},
		{Name: "migration2", Optional: true},
		{Name: "migration3", Optional: true, Ran: true},
	})
}
