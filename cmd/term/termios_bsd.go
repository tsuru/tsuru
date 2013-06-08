// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin

package term

import "syscall"

var (
	TCGETS uintptr = syscall.TIOCGETA
	TCSETS uintptr = syscall.TIOCSETA
)
