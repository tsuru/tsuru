// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

import (
	/* "fmt" */
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	/* "runtime" */
	/* "sync" */
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

var _ = gocheck.Suite(Suite{})

type Suite struct{}

func (Suite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_quota_tests")
}

func (Suite) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (Suite) TestCreate(c *gocheck.C) {
	err := Create("user@tsuru.io", 10)
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	var u usage
	err = conn.Quota().Find(bson.M{"owner": "user@tsuru.io"}).One(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Quota().Remove(bson.M{"owner": "user@tsuru.io"})
	c.Assert(u.Owner, gocheck.Equals, "user@tsuru.io")
	c.Assert(u.Limit, gocheck.Equals, uint(10))
	c.Assert(u.Items, gocheck.HasLen, 0)
}

func (Suite) TestDuplicateQuota(c *gocheck.C) {
	err := Create("user@tsuru.io", 10)
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer conn.Quota().Remove(bson.M{"owner": "user@tsuru.io"})
	err = Create("user@tsuru.io", 50)
	c.Assert(err, gocheck.Equals, ErrQuotaAlreadyExists)
}

func (Suite) TestDelete(c *gocheck.C) {
	err := Create("home@dreamtheater.com", 3)
	c.Assert(err, gocheck.IsNil)
	err = Delete("home@dreamtheater.com")
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	count, err := conn.Quota().Find(bson.M{"owner": "home@dreamtheater.com"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (Suite) TestDeleteQuotaNotFound(c *gocheck.C) {
	err := Delete("home@dreamtheater.com")
	c.Assert(err, gocheck.Equals, ErrQuotaNotFound)
}

func (Suite) TestReserve(c *gocheck.C) {
	err := Create("last@dreamtheater.com", 3)
	c.Assert(err, gocheck.IsNil)
	defer Delete("last@dreamtheater.com")
	err = Reserve("last@dreamtheater.com", "dt/1")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("last@dreamtheater.com", "dt/0")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("last@dreamtheater.com", "dt/0")
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	var u usage
	err = conn.Quota().Find(bson.M{"owner": "last@dreamtheater.com"}).One(&u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Items, gocheck.DeepEquals, []string{"dt/1", "dt/0"})
}

// func (Suite) TestReserveIsSafe(c *gocheck.C) {
// 	items := 300
// 	err := Create("spirit@dreamtheater.com", uint(items-items/2))
// 	c.Assert(err, gocheck.IsNil)
// 	defer Delete("spirit@dreamtheater.com")
// 	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(8))
// 	var wg sync.WaitGroup
// 	wg.Add(items)
// 	for i := 0; i < items; i++ {
// 		go func(i int) {
// 			Reserve("spirit@dreamtheater.com", fmt.Sprintf("spirit/%d", i))
// 			wg.Done()
// 		}(i)
// 	}
// 	wg.Wait()
// 	conn, err := db.Conn()
// 	c.Assert(err, gocheck.IsNil)
// 	defer conn.Close()
// 	var u usage
// 	err = conn.Quota().Find(bson.M{"owner": "spirit@dreamtheater.com"}).One(&u)
// 	c.Assert(err, gocheck.IsNil)
// 	c.Assert(u.Items, gocheck.HasLen, items-items/2)
// }

func (Suite) TestReserveRepeatedItems(c *gocheck.C) {
	err := Create("spirit@dreamtheater.com", 500)
	c.Assert(err, gocheck.IsNil)
	defer Delete("spirit@dreamtheater.com")
	err = Reserve("spirit@dreamtheater.com", "spirit/0")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("spirit@dreamtheater.com", "spirit/0")
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	var u usage
	err = conn.Quota().Find(bson.M{"owner": "spirit@dreamtheater.com"}).One(&u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Items, gocheck.HasLen, 1)
}

func (Suite) TestReserveMultiple(c *gocheck.C) {
	err := Create("barbarian@elp.com", 3)
	c.Assert(err, gocheck.IsNil)
	defer Delete("barbarian@elp.com")
	err = Reserve("barbarian@elp.com", "b/0", "b/1", "b/2")
	c.Assert(err, gocheck.IsNil)
}

func (Suite) TestReserveMultipleExceed(c *gocheck.C) {
	err := Create("pebble@elp.com", 2)
	c.Assert(err, gocheck.IsNil)
	defer Delete("pebble@elp.com")
	err = Reserve("pebble@elp.com", "p/0", "p/1", "p/2")
	e, ok := err.(*QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Available, gocheck.Equals, uint(2))
	c.Assert(e.Requested, gocheck.Equals, uint(3))
}

func (Suite) TestReserveQuotaExceeded(c *gocheck.C) {
	err := Create("change@dreamtheater.com", 1)
	c.Assert(err, gocheck.IsNil)
	defer Delete("change@dreamtheater.com")
	err = Reserve("change@dreamtheater.com", "change/0")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("change@dreamtheater.com", "change/1")
	e, ok := err.(*QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Available, gocheck.Equals, uint(0))
	c.Assert(e.Requested, gocheck.Equals, uint(1))
}

func (Suite) TestReserveQuotaNotFound(c *gocheck.C) {
	err := Reserve("home@dreamtheater.com", "something")
	c.Assert(err, gocheck.Equals, ErrQuotaNotFound)
}

func (Suite) TestRelease(c *gocheck.C) {
	err := Create("beyond@yes.com", 1)
	c.Assert(err, gocheck.IsNil)
	defer Delete("beyond@yes.com")
	err = Reserve("beyond@yes.com", "beyond/0")
	c.Assert(err, gocheck.IsNil)
	err = Release("beyond@yes.com", "beyond/0")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("beyond@yes.com", "beyond/1")
	c.Assert(err, gocheck.IsNil)
}

func (Suite) TestReleaseQuotaNotFound(c *gocheck.C) {
	err := Release("see@yes.com", "see/0")
	c.Assert(err, gocheck.Equals, ErrQuotaNotFound)
}

// func (Suite) TestReleaseIsSafe(c *gocheck.C) {
// 	items := 50
// 	err := Create("looking@yes.com", uint(items))
// 	c.Assert(err, gocheck.IsNil)
// 	defer Delete("looking@yes.com")
// 	for i := 0; i < items; i++ {
// 		Reserve("looking@yes.com", fmt.Sprintf("looking/%d", i))
// 	}
// 	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(8))
// 	var wg sync.WaitGroup
// 	for i := 0; i < items+items/2; i++ {
// 		wg.Add(1)
// 		go func(i int) {
// 			err := Release("looking@yes.com", fmt.Sprintf("looking/%d", i))
// 			c.Check(err, gocheck.IsNil)
// 			wg.Done()
// 		}(i)
// 	}
// 	wg.Wait()
// 	conn, err := db.Conn()
// 	c.Assert(err, gocheck.IsNil)
// 	defer conn.Close()
// 	var u usage
// 	err = conn.Quota().Find(bson.M{"owner": "looking@yes.com"}).One(&u)
// 	c.Assert(err, gocheck.IsNil)
// 	c.Assert(u.Items, gocheck.HasLen, 0)
// }

func (Suite) TestReleaseMultiple(c *gocheck.C) {
	err := Create("tank@elp.com", 3)
	c.Assert(err, gocheck.IsNil)
	defer Delete("tank@elp.com")
	err = Reserve("tank@elp.com", "tank/0", "tank/1", "tank/2")
	c.Assert(err, gocheck.IsNil)
	err = Release("tank@elp.com", "tank/0", "tank/2")
	c.Assert(err, gocheck.IsNil)
	items, available, err := Items("tank@elp.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(items, gocheck.DeepEquals, []string{"tank/1"})
	c.Assert(available, gocheck.Equals, uint(2))
}

func (Suite) TestSetQuota(c *gocheck.C) {
	err := Create("survival@yes.com", 1)
	c.Assert(err, gocheck.IsNil)
	defer Delete("survival@yes.com")
	err = Reserve("survival@yes.com", "survival/0")
	c.Assert(err, gocheck.IsNil)
	err = Set("survival@yes.com", 10)
	c.Assert(err, gocheck.IsNil)
	err = Reserve("survival@yes.com", "survival/1")
	c.Assert(err, gocheck.IsNil)
}

func (Suite) TestSetQuotaNotFound(c *gocheck.C) {
	err := Set("everydays@yes.com", 10)
	c.Assert(err, gocheck.Equals, ErrQuotaNotFound)
}

func (Suite) TestItemsGreaterThanLimit(c *gocheck.C) {
	err := Create("coming@yes.com", 3)
	c.Assert(err, gocheck.IsNil)
	defer Delete("coming@yes.com")
	err = Reserve("coming@yes.com", "coming/0")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("coming@yes.com", "coming/1")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("coming@yes.com", "coming/2")
	c.Assert(err, gocheck.IsNil)
	err = Set("coming@yes.com", 2)
	c.Assert(err, gocheck.IsNil)
	err = Reserve("coming@yes.com", "coming/3")
	e, ok := err.(*QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Available, gocheck.Equals, uint(0))
	c.Assert(e.Requested, gocheck.Equals, uint(1))
}

func (Suite) TestItems(c *gocheck.C) {
	err := Create("sorry@evergrey.com", 4)
	c.Assert(err, gocheck.IsNil)
	defer Delete("sorry@evergrey.com")
	items, available, err := Items("sorry@evergrey.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(available, gocheck.Equals, uint(4))
	c.Assert(items, gocheck.HasLen, 0)
	err = Reserve("sorry@evergrey.com", "sorry/0")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("sorry@evergrey.com", "sorry/1")
	c.Assert(err, gocheck.IsNil)
	items, available, err = Items("sorry@evergrey.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(items, gocheck.DeepEquals, []string{"sorry/0", "sorry/1"})
	c.Assert(available, gocheck.Equals, uint(2))
	err = Reserve("sorry@evergrey.com", "sorry/2")
	c.Assert(err, gocheck.IsNil)
	err = Reserve("sorry@evergrey.com", "sorry/3")
	c.Assert(err, gocheck.IsNil)
	items, available, err = Items("sorry@evergrey.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(available, gocheck.Equals, uint(0))
	c.Assert(items, gocheck.DeepEquals, []string{"sorry/0", "sorry/1", "sorry/2", "sorry/3"})
}

func (Suite) TestItemsAvailableNegative(c *gocheck.C) {
	err := Create("lie@evergrey.com", 1)
	c.Assert(err, gocheck.IsNil)
	defer Delete("lie@evergrey.com")
	err = Reserve("lie@evergrey.com", "lie/0")
	c.Assert(err, gocheck.IsNil)
	err = Set("lie@evergrey.com", 0)
	c.Assert(err, gocheck.IsNil)
	_, available, err := Items("lie@evergrey.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(available, gocheck.Equals, uint(0))
}

func (Suite) TestItemsQuotaNotFound(c *gocheck.C) {
	items, available, err := Items("blinded@evergrey.com")
	c.Assert(items, gocheck.IsNil)
	c.Assert(available, gocheck.Equals, uint(0))
	c.Assert(err, gocheck.Equals, ErrQuotaNotFound)
}

func (Suite) TestQuotaExceededError(c *gocheck.C) {
	err := QuotaExceededError{Requested: 10, Available: 9}
	c.Assert(err.Error(), gocheck.Equals, "Quota exceeded. Available: 9. Requested: 10.")
}
