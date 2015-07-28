// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fstest

import (
	"os"
	"time"

	"gopkg.in/check.v1"
)

func (s *S) TestFileInfo(c *check.C) {
	now := time.Now()
	sysInfo := &now
	var fi os.FileInfo = &fileInfo{
		name:    "/home/application/apprc",
		size:    104857600,
		mode:    0644,
		modTime: now,
		isDir:   false,
		sys:     sysInfo,
	}
	c.Check(fi.Name(), check.Equals, "/home/application/apprc")
	c.Check(fi.Size(), check.Equals, int64(104857600))
	c.Check(fi.Mode(), check.Equals, os.FileMode(0644))
	c.Check(fi.ModTime(), check.DeepEquals, now)
	c.Check(fi.IsDir(), check.Equals, false)
	c.Check(fi.Sys(), check.Equals, sysInfo)
}
