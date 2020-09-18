// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache_test

import (
	"context"
	"fmt"
	"net/url"

	"github.com/tsuru/tsuru/router"
	_ "github.com/tsuru/tsuru/router/hipache"
	"github.com/tsuru/tsuru/router/routertest"
)

func Example() {
	router, err := router.Get(context.TODO(), "hipache")
	if err != nil {
		panic(err)
	}
	err = router.AddBackend(routertest.FakeApp{Name: "myapp"})
	if err != nil {
		panic(err)
	}
	u, err := url.Parse("http://10.10.10.10:8080")
	if err != nil {
		panic(err)
	}
	err = router.AddRoutes("myapp", []*url.URL{u})
	if err != nil {
		panic(err)
	}
	addr, _ := router.Addr("myapp")
	fmt.Println("Please access:", addr)
	err = router.RemoveRoutes("myapp", []*url.URL{u})
	if err != nil {
		panic(err)
	}
	err = router.RemoveBackend("myapp")
	if err != nil {
		panic(err)
	}
}
