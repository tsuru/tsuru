// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elb

import "github.com/globocom/tsuru/router"

func init() {
	router.Register("elb", elbRouter{})
}

type elbRouter struct{}

func (elbRouter) AddBackend(name string) error {
	return nil
}

func (elbRouter) RemoveBackend(name string) error {
	return nil
}

func (elbRouter) AddRoute(name, address string) error {
	return nil
}

func (elbRouter) RemoveRoute(name, address string) error {
	return nil
}

func (elbRouter) SetCName(cname, name string) error {
	return nil
}

func (elbRouter) UnsetCName(cname, name string) error {
	return nil
}

func (elbRouter) Addr(name string) (string, error) {
	return "", nil
}
