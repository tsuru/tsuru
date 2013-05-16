// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bufio"
	"bytes"
	"io"
)

type filter struct {
	w        io.Writer
	content  []byte
	filtered bool
}

func (f *filter) Write(p []byte) (int, error) {
	var written int
	reader := bufio.NewReader(bytes.NewReader(p))
	line, _ := reader.ReadBytes('\n')
	for len(line) > 0 {
		if !bytes.Contains(bytes.ToLower(line), f.content) {
			n, err := f.w.Write(line)
			written += n
			if err != nil {
				return written, err
			}
		} else {
			f.filtered = true
		}
		line, _ = reader.ReadBytes('\n')
	}
	return len(p), nil
}
