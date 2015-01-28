// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package repository contains types and function for git repository
// interaction.
package repository

import (
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/log"
)

var ErrGandalfDisabled = errors.New("git server is disabled")

// ServerURL returns the URL to Gandalf API.
func ServerURL() (string, error) {
	server, err := config.GetString("git:api-server")
	if err != nil {
		return "", ErrGandalfDisabled
	}
	return server, nil
}

// ReadWriteURL returns the SSH URL, for writing and reading operations.
func ReadWriteURL(app string) string {
	serverURL, err := ServerURL()
	if err != nil {
		log.Errorf("Error retrieving repository: %s", err)
		return "<none>"
	}
	c := gandalf.Client{Endpoint: serverURL}
	repository, err := c.GetRepository(app)
	if err != nil {
		log.Errorf("Caught error while retrieving repository: %s", err.Error())
		return ""
	}
	return repository.SshURL
}

// ReadOnlyURL returns the url for communication with git-daemon.
func ReadOnlyURL(app string) string {
	serverURL, err := ServerURL()
	if err != nil {
		log.Errorf("Error retrieving repository: %s", err)
		return "<none>"
	}
	c := gandalf.Client{Endpoint: serverURL}
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
