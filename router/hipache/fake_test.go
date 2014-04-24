// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
	"github.com/garyburd/redigo/redis"
	rtesting "github.com/tsuru/tsuru/testing/redis"
)

var conn redis.Conn = &rtesting.FakeRedisConn{}

func fakeConnect() (redis.Conn, error) {
	return conn, nil
}
