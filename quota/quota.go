// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package quota provides primitives for quota management in Tsuru.
package quota

import "fmt"

var Unlimited = Quota{Limit: -1, InUse: 0}

type Quota struct {
	Limit int
	InUse int
}
type QuotaExceededError struct {
	Requested uint
	Available uint
}

func (err *QuotaExceededError) Error() string {
	return fmt.Sprintf("Quota exceeded. Available: %d. Requested: %d.", err.Available, err.Requested)
}
