// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"launchpad.net/gocheck"
	"testing"
)

const runs = 50

func check(b *testing.B, err error) {
	if err != nil {
		b.Errorf("Failed to create user: %s", err.Error())
		b.FailNow()
	}
}

func BenchmarkAddKeyToUserAlwaysSetTheKeyName(b *testing.B) {
	b.StopTimer()

	s := &S{}
	s.SetUpSuite(&gocheck.C{})
	defer s.TearDownSuite(&gocheck.C{})

	u := &User{Email: "issue9@timeredbull.com", Password: "123"}
	err := u.Create()
	check(b, err)
	err = createTeam("issue9team", u)
	check(b, err)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("my-key-%d", i+1)
		err = addKeyToUser(key, u)
		check(b, err)
	}
	err = u.Get()
	check(b, err)
	if len(u.Keys) != b.N {
		b.Errorf("Did not save all keys!")
		b.FailNow()
	}
	count := 0
	for _, key := range u.Keys {
		if key.Name == "" {
			count++
		}
	}
	if count > 0 {
		b.Errorf("%d keys without name", count)
	}
}
