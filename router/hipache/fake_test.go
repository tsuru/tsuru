// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
	"github.com/garyburd/redigo/redis"
	"github.com/tsuru/tsuru/testing"
)

var conn redis.Conn = &testing.FakeRedisConn{}

func fakeConnect() (redis.Conn, error) {
	return conn, nil
}
