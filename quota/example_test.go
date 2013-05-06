// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota_test

import (
	"github.com/globocom/tsuru/quota"
)

func ExampleReserve() {
	err := quota.Create("me@tsuru.io", 4)
	if err != nil {
		panic(err)
	}
	quota.Reserve("me@tsuru.io", "me/0", "me/1", "me/2") // ok
	quota.Reserve("me@tsuru.io", "me/3", "me/4", "me/5") // ErrQuotaExceeded
}

func ExampleSet() {
	err := quota.Create("me@tsuru.io", 3)
	if err != nil {
		panic(err)
	}
	quota.Reserve("me@tsuru.io", "me/0")
	quota.Reserve("me@tsuru.io", "me/1")
	quota.Reserve("me@tsuru.io", "me/2")
	quota.Set("me@tsuru.io", 2)
	quota.Reserve("me@tsuru.io", "me/3") // ErrQuotaExceeded
	quota.Release("me@tsuru.io", "me/2")
	quota.Reserve("me@tsuru.io", "me/3") // ErrQuotaExceeded
	quota.Release("me@tsuru.io", "me/1")
	quota.Reserve("me@tsuru.io", "me/3") // Everything is ok now
}
