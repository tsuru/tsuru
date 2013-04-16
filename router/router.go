// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package router provides interfaces that need to be satisfied in order to
// implement a new router on tsuru.
package router

// Router is the basic interface of this package.
type Router interface {
	// AddRoute addes a new route.
	AddRoute(name, ip string) error

	//Remove removes a route.
	RemoveRoute(name string) error

	// Restart restarts the router.
	Restart() error
}
