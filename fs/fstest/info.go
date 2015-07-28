// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fstest

import (
	"os"
	"time"
)

// FileInfo provides an attribute based implementation of the os.FileInfo
// interface.
type FileInfo struct {
	FileName    string
	FileSize    int64
	FileMode    os.FileMode
	FileModTime time.Time
	FileIsDir   bool
	FileSys     interface{}
}

func (fi *FileInfo) Name() string {
	return fi.FileName
}

func (fi *FileInfo) Size() int64 {
	return fi.FileSize
}

func (fi *FileInfo) Mode() os.FileMode {
	return fi.FileMode
}

func (fi *FileInfo) ModTime() time.Time {
	return fi.FileModTime
}

func (fi *FileInfo) IsDir() bool {
	return fi.FileIsDir
}

func (fi *FileInfo) Sys() interface{} {
	return fi.FileSys
}
