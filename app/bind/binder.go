// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bind provides interfaces and types for use when binding an app to a
// service.
package bind

import "fmt"

type EnvVar struct {
	Name         string
	Value        string
	Public       bool
	InstanceName string
}

func (e *EnvVar) String() string {
	var value, suffix string
	if e.Public {
		value = e.Value
	} else {
		value = "***"
		suffix = " (private variable)"
	}
	return fmt.Sprintf("%s=%s%s", e.Name, value, suffix)
}

type Unit interface {
	GetIp() string
}

type App interface {
	GetName() string
	GetUnits() []Unit
	InstanceEnv(string) map[string]EnvVar
	SetEnvs([]EnvVar, bool) error
	UnsetEnvs([]string, bool) error
}

type Binder interface {
	Bind(App) error
	Unbind(App) error
}
