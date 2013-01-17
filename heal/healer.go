// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package heal provides an interface for heal anything.
package heal

// Healer represents a healer.
type Healer interface {
	// Check verifies if something is need a heal.
	Check() bool

	// Heal heals something.
	Heal() error
}
