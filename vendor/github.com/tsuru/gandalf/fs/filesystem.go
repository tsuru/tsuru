// Copyright 2014 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// fs is just a filesystem wrapper.
// It makes use of tsuru/fs pkg.
package fs

import (
	"github.com/tsuru/tsuru/fs"
)

var Fsystem fs.Fs

func Filesystem() fs.Fs {
	if Fsystem == nil {
		return fs.OsFs{}
	}
	return Fsystem
}
