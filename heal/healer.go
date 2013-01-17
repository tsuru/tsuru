// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package heal provides an interface for heal anything.
package heal

import "fmt"

// Healer represents a healer.
type Healer interface {
	// NeedsHeal verifies if something needs the heal.
	NeedsHeal() bool

	// Heal heals something.
	Heal() error
}

var healers = make(map[string]Healer)

// Register registers a new healer in the Healer registry.
func Register(name string, h Healer) {
	healers[name] = h
}

// Get gets the named healer from the registry.
func Get(name string) (Healer, error) {
	h, ok := healers[name]
	if !ok {
		return nil, fmt.Errorf("Unknown healer: %q.", name)
	}
	return h, nil
}
