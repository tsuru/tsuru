// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"os"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagepg"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type postgresBaseTest struct{}

// testDatabaseURL points at the docker-compose `postgres` service. Override it
// with TSURU_POSTGRES_TEST_URL when the default port is taken locally.
func testDatabaseURL() string {
	if url := os.Getenv("TSURU_POSTGRES_TEST_URL"); url != "" {
		return url
	}
	return storagepg.DefaultDatabaseURL
}

func (t *postgresBaseTest) SetUpSuite(c *check.C) {
	config.Set("database:postgres:url", testDatabaseURL())
	storagepg.Reset()
	// Force a connect so the schema is created before the first test.
	_, err := storagepg.Pool()
	c.Assert(err, check.IsNil)
}

func (t *postgresBaseTest) SetUpTest(c *check.C) {
	err := storagepg.ClearAllTables()
	c.Assert(err, check.IsNil)
}

func (t *postgresBaseTest) TearDownSuite(c *check.C) {
	err := storagepg.ClearAllTables()
	c.Assert(err, check.IsNil)
}

func (t *postgresBaseTest) TearDownTest(c *check.C) {}
