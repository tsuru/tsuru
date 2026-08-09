// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagepg

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tsuru/config"
)

const DefaultDatabaseURL = "postgres://tsuru:tsuru@127.0.0.1:5432/tsuru?sslmode=disable"

var (
	pool *pgxpool.Pool
	mu   sync.Mutex
)

// schema is applied idempotently on first connect. The JSONB document-mirror
// stores the full entity in `doc`; promoted columns back hot predicates.
const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	name       text PRIMARY KEY,
	team_owner text,
	owner      text,
	pool       text,
	tags       text[],
	doc        jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS jobs_pool_idx ON jobs (pool);
CREATE INDEX IF NOT EXISTS jobs_team_owner_idx ON jobs (team_owner);
`

func dbURL() string {
	url, _ := config.GetString("database:postgres:url")
	if url == "" {
		url = DefaultDatabaseURL
	}
	return url
}

func connect() (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	p, err := pgxpool.New(ctx, dbURL())
	if err != nil {
		return nil, err
	}
	if _, err = p.Exec(ctx, schema); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

// Pool returns the shared connection pool, connecting on first use.
func Pool() (*pgxpool.Pool, error) {
	mu.Lock()
	defer mu.Unlock()
	if pool != nil {
		return pool, nil
	}
	p, err := connect()
	if err != nil {
		return nil, err
	}
	pool = p
	return pool, nil
}

// Reset drops the cached pool (used between test suites).
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	if pool != nil {
		pool.Close()
		pool = nil
	}
}
