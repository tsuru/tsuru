// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rec_test

import (
	"github.com/tsuru/tsuru/rec"
)

func ExampleLog() {
	// logging and waiting
	ch := rec.Log("user@email.com", "action", "arg1", 10, true)
	<-ch

	// logging without blocking
	rec.Log("user@email.com", "action-2", "arg1", 10, true)

	// logging and checking for errors
	ch = rec.Log("user@email.com", "action-3", "arg1", 10, true)
	if err, ok := <-ch; ok {
		panic(err)
	}
}
