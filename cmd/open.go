// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows,!linux,!darwin

package cmd

import "fmt"

func open(url string) error {
	return fmt.Errorf("cannot open %s", url)
}
