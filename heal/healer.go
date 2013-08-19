// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package heal provides an interface for heal anything.
package heal

import "fmt"

// Healer represents a healer.
type Healer interface {
	// Heal heals something.
	Heal() error
}

var healers = make(map[string]map[string]Healer)

// Register registers a new healer in the Healer registry.
func Register(provisioner, name string, h Healer) {
	if _, ok := healers[provisioner]; !ok {
		healers[provisioner] = map[string]Healer{name: h}
	} else {
		healers[provisioner][name] = h
	}
}

// Get gets the named healer from the registry.
func Get(provisioner, name string) (Healer, error) {
	h, ok := healers[provisioner][name]
	if !ok {
		return nil, fmt.Errorf("Unknown healer %q for provisioner %q.", name, provisioner)
	}
	return h, nil
}

func All(provisioner string) map[string]Healer {
	return healers[provisioner]
}
