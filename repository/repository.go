// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package repository contains types and function for git repository
// interaction.
package repository

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/log"
)

// ServerURL returns the URL to Gandalf API.
func ServerURL() string {
	server, err := config.GetString("git:api-server")
	if err != nil {
		log.Error("git:api-server config not found")
		panic(err)
	}
	return server
}

// ReadWriteURL returns the SSH URL, for writing and reading operations.
func ReadWriteURL(app string) string {
	publicHost, err := config.GetString("git:rw-host")
	if err != nil {
		log.Error("git:rw-host config not found")
		panic(err)
	}
	return fmt.Sprintf("git@%s:%s.git", publicHost, app)
}

// ReadOnlyURL returns the url for communication with git-daemon.
func ReadOnlyURL(app string) string {
	roHost, err := config.GetString("git:ro-host")
	if err != nil {
		log.Error("git:ro-host config not found")
		panic(err)
	}
	return fmt.Sprintf("git://%s/%s.git", roHost, app)
}

// GetPath returns the path to the repository where the app code is in its
// units.
func GetPath() (string, error) {
	return config.GetString("git:unit-repo")
}