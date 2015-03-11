// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package repository contains types and functions for git repository
// interaction.
package repository

import (
	"errors"

	"github.com/tsuru/config"
)

const defaultManager = "gandalf"

var managers map[string]RepositoryManager

var (
	ErrKeyNotFound            = errors.New("key not found")
	ErrRepositoryNotFound     = errors.New("repository not found")
	ErrUserNotFound           = errors.New("user not found")
	ErrKeyAlreadyExists       = errors.New("user already have this key")
	ErrRepositoryAlreadExists = errors.New("repository already exists")
	ErrUserAlreadyExists      = errors.New("user already exists")
)

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

// Initializer is a RepositoryManager that can be initialized. tsuru will call
// the Initialize method on startup, whenever the "repo-manager" points to a
// Initializer.
//
// tsuru will abort startup in case of a failure in the Initialize method.
type Initializer interface {
	Initialize() error
}

// RepositoryManager represents a manager of application repositories.
type RepositoryManager interface {
	CreateUser(username string) error
	RemoveUser(username string) error

	GrantAccess(repository, user string) error
	RevokeAccess(repository, user string) error

	CreateRepository(name string, users []string) error
	RemoveRepository(name string) error
	GetRepository(name string) (Repository, error)

	Diff(repositoryName, fromVersion, toVersion string) (string, error)
}

// KeyRepositoryManager is a RepositoryManager that is able to manage public
// SSH keys.
type KeyRepositoryManager interface {
	AddKey(username string, key Key) error
	RemoveKey(username string, key Key) error
	ListKeys(username string) ([]Key, error)
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
