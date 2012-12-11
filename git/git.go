// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package git provides types and utilities for dealing with Git repositories.
// It's very limited, and provide some access to git config file, being focused
// on tsuru needs.
package git

import (
	"errors"
	"os"
	"path"
)

// DiscoverRepositoryPath finds the path of the repository from a given
// directory. It returns the path to the repository, or an an empty string and
// a non-nil error if it can't find the repository.
func DiscoverRepositoryPath(dir string) (string, error) {
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return "", errors.New("Repository not found.")
	}
	dir = path.Join(dir, ".git")
	for dir != "/.git" {
		fi, err := os.Stat(dir)
		if err == nil && fi.IsDir() {
			return dir, nil
		}
		dir = path.Join(dir, "..", "..", ".git")
	}
	return "", errors.New("Repository not found.")
}
