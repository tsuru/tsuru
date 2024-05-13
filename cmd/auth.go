// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
	terminal "golang.org/x/term"
)

func PasswordFromReader(reader io.Reader) (string, error) {
	var (
		password []byte
		err      error
	)
	if desc, ok := reader.(descriptable); ok && terminal.IsTerminal(int(desc.Fd())) {
		password, err = terminal.ReadPassword(int(desc.Fd()))
		if err != nil {
			return "", err
		}
	} else {
		fmt.Fscanf(reader, "%s\n", &password)
	}
	if len(password) == 0 {
		msg := "You must provide the password!"
		return "", errors.New(msg)
	}
	return string(password), err
}
