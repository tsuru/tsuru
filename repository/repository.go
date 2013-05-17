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

// GitServerUri returns the URL to Gandalf API.
func GitServerUri() string {
	server, err := config.GetString("git:api-server")
	if err != nil {
		log.Print("git:api-server config not found")
		panic(err)
	}
	return server
}

// GetUrl returns the ssh clone-url from an app.
func GetUrl(app string) string {
	publicHost, err := config.GetString("git:rw-host")
	if err != nil {
		log.Print("git:rw-host config not found")
		panic(err)
	}
	return fmt.Sprintf("git@%s:%s.git", publicHost, app)
}

// GetReadOnlyUrl returns the url for communication with git-daemon.
func GetReadOnlyUrl(app string) string {
	roHost, err := config.GetString("git:ro-host")
	if err != nil {
		log.Print("git:ro-host config not found")
		panic(err)
	}
	return fmt.Sprintf("git://%s/%s.git", roHost, app)
}

// GetPath returns the path to the repository where the app code is in its
// units.
func GetPath() (string, error) {
	return config.GetString("git:unit-repo")
}
