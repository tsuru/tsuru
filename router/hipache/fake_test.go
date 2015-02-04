// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import "github.com/garyburd/redigo/redis"

var conn redis.Conn = &FakeRedisConn{}

func fakeConnect() (redis.Conn, error) {
	return conn, nil
}
