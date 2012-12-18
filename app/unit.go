// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/tsuru/provision"
)

type Unit struct {
	Name    string
	Type    string
	Machine int
	Ip      string
	State   string
	app     *App
}

func (u *Unit) GetName() string {
	return u.Name
}

func (u *Unit) GetMachine() int {
	return u.Machine
}

func (u *Unit) GetIp() string {
	return u.Ip
}

func (u *Unit) GetStatus() provision.Status {
	return provision.Status(u.State)
}
