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

// getGitServer returns the git server defined in the tsuru.conf file.
//
// If git:host configuration is not defined, this function panics.
func getGitServer() string {
	gitServer, err := config.GetString("git:host")
	if err != nil {
		log.Print("git:host config not found")
		panic(err)
	}
	return gitServer
}

//  joins the protocol, server and port together and returns.
//
// This functions makes uses of three configurations:
//   - git:host
//   - git:protocol
//   - git:port (optional)
//
// If some of the required configuration is not found, this function panics.
func GitServerUri() string {
	server, err := config.GetString("git:host")
	if err != nil {
		log.Print("git:host config not found")
		panic(err)
	}
	protocol, _ := config.GetString("git:protocol")
	if protocol == "" {
		protocol = "http"
	}
	uri := fmt.Sprintf("%s://%s", protocol, server)
	if port, err := config.Get("git:port"); err == nil {
		uri = fmt.Sprintf("%s:%d", uri, port)
	}
	return uri
}

// GetUrl returns the ssh clone-url from an app.
func GetUrl(app string) string {
	return fmt.Sprintf("git@%s:%s.git", getGitServer(), app)
}

// GetReadOnlyUrl returns the url for communication with git-daemon.
func GetReadOnlyUrl(app string) string {
	return fmt.Sprintf("git://%s/%s.git", getGitServer(), app)
}

// GetPath returns the path to the repository where the app code is in its
// units.
func GetPath() (string, error) {
	return config.GetString("git:unit-repo")
}
