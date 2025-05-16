// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"time"
)

type AppLock struct {
	Locked      bool
	Reason      string
	Owner       string
	AcquireDate time.Time
}

type AppLockInterface interface {
	String()
}

func (l *AppLock) String() string {
	format := `Lock:
 Acquired in: %s
 Owner: %s
 Running: %s`
	return fmt.Sprintf(format, l.AcquireDate, l.Owner, l.Reason)
}
