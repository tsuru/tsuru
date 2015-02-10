// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package repository contains types and function for git repository
// interaction.
package repository

import "github.com/tsuru/config"

const defaultManager = "gandalf"

var managers map[string]RepositoryManager

// Key represents a public key, that is added to a repository to allow access
// to it.
type Key struct {
	Name string
	Body string
}

// Repository represents a repository in the manager.
type Repository struct {
	Name         string
	ReadOnlyURL  string
	ReadWriteURL string
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

	CreateRepository(name string) error
	RemoveRepository(name string) error
	GetRepository(name string) (Repository, error)
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
