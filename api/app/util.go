// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"io"
)

func randomBytes(n int) ([]byte, error) {
	f, err := filesystem().Open("/dev/urandom")
	if err != nil {
		return nil, err
	}
	b := make([]byte, n)
	read, err := f.Read(b)
	if err != nil {
		return nil, err
	}
	if read != n {
		return nil, io.ErrShortBuffer
	}
	err = f.Close()
	if err != nil {
		return nil, err
	}
	return b, nil
}

// newUUID generates an uuid.
func newUUID() (string, error) {
	b, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	uuid := fmt.Sprintf("%x", b)
	return uuid, nil
}
