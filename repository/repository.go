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
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/log"
)

const defaultManager = "gandalf"

func init() {
	hc.AddChecker("Gandalf", healthCheck)
}

func healthCheck() error {
	serverURL, err := ServerURL()
	if err == ErrGandalfDisabled {
		return hc.ErrDisabledComponent
	}
	client := gandalf.Client{Endpoint: serverURL}
	result, err := client.GetHealthCheck()
	if err != nil {
		return err
	}
	status := string(result)
	if status == "WORKING" {
		return nil
	}
	return errors.New("unexpected status - " + status)
}

var ErrGandalfDisabled = errors.New("git server is disabled")

var managers map[string]RepositoryManager

// Key represents a public key, that is added to a repository to allow access
// to it.
type Key struct {
	Name string
	Body string
}

// RepositoryManager represents a manager of application repositories.
type RepositoryManager interface {
	CreateUser(username string) error
	RemoveUser(username string) error

	GrantAccess(repository, user string) error
	RevokeAccess(repository, user string) error

	AddKey(username string, key Key) error
	RemoveKey(username string, key Key) error
	ListKeys(username string) ([]Key, error)

	ReadOnlyURL(repository string) (string, error)
	ReadWriteURL(repository string) (string, error)
}

// Manager returns the current configured manager, as defined in the
// configuration file.
func Manager() RepositoryManager {
	managerName, err := config.GetString("repo-manager")
	if err != nil {
		managerName = defaultManager
	}
	if _, ok := managers[managerName]; !ok {
		managerName = "nop"
	}
	return managers[managerName]
}

// Register registers a new repository manager, that can be later configured
// and used.
func Register(name string, manager RepositoryManager) {
	if managers == nil {
		managers = make(map[string]RepositoryManager)
	}
	managers[name] = manager
}

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
