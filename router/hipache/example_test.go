// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache_test

import (
	"context"
	"fmt"
	"net/url"

	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router"
	_ "github.com/tsuru/tsuru/router/hipache"
	"github.com/tsuru/tsuru/router/routertest"
)

func Example() {
	ctx := context.TODO()
	router, err := router.Get(ctx, "hipache")
	if err != nil {
		panic(err)
	}
	err = router.AddBackend(ctx, routertest.FakeApp{Name: "myapp"})
	if err != nil {
		panic(err)
	}
	u, err := url.Parse("http://10.10.10.10:8080")
	if err != nil {
		panic(err)
	}
	app := provisiontest.NewFakeApp("myapp", "static", 4)
	err = router.AddRoutes(ctx, app, []*url.URL{u})
	if err != nil {
		panic(err)
	}
	addr, _ := router.Addr(ctx, app)
	fmt.Println("Please access:", addr)
	err = router.RemoveRoutes(ctx, app, []*url.URL{u})
	if err != nil {
		panic(err)
	}
	err = router.RemoveBackend(ctx, app)
	if err != nil {
		panic(err)
	}
}
