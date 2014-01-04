// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package repository contains types and function for git repository
// interaction.
package repository

import (
	"github.com/globocom/config"
	"github.com/globocom/go-gandalfclient"
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
	c := gandalf.Client{Endpoint: ServerURL()}
	repository, err := c.GetRepository(app)
	if err != nil {
		log.Errorf("Caught error while retrieving repository: %s", err.Error())
		return ""
	}
	return repository.SshURL
}

// ReadOnlyURL returns the url for communication with git-daemon.
func ReadOnlyURL(app string) string {
	c := gandalf.Client{Endpoint: ServerURL()}
	repository, err := c.GetRepository(app)
	if err != nil {
		log.Errorf("Caught error while retrieving repository: %s", err.Error())
		return ""
	}
	return repository.GitURL
}

// GetPath returns the path to the repository where the app code is in its
// units.
func GetPath() (string, error) {
	return config.GetString("git:unit-repo")
}
