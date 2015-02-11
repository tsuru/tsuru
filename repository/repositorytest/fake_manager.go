// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package repositorytest provides a fake repository manager for use in tests.
//
// Users can use the fake manager by just importing this package, setting the
// "repo-manager" setting to "fake" and interacting with the manager.
//
// This package also includes some helper functions that allow to interact with
// the unexported state of the fake manager, allowing users of the package to
// reset the manager or retrieve the list of registered users, for example.
package repositorytest

import (
	"errors"
	"fmt"
	"sync"

	"github.com/tsuru/tsuru/repository"
)

func init() {
	repository.Register("fake", &manager)
}

// ServerHost is the name of the host that is used in the Git URLs in all
// repositories managed by the fake manager.
const ServerHost = "git.tsuru.io"

var manager = fakeManager{grants: make(map[string][]string), keys: make(map[string]map[string]string)}

type fakeManager struct {
	grants     map[string][]string
	grantsLock sync.Mutex
	keys       map[string]map[string]string
	keysLock   sync.Mutex
}

func (m *fakeManager) CreateUser(username string) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	if _, ok := m.keys[username]; ok {
		return errors.New("user already exists")
	}
	m.keys[username] = make(map[string]string)
	return nil
}

func (m *fakeManager) RemoveUser(username string) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	if _, ok := m.keys[username]; !ok {
		return errors.New("user not found")
	}
	delete(m.keys, username)
	return nil
}

func (m *fakeManager) CreateRepository(name string) error {
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[name]; ok {
		return errors.New("repository already exists")
	}
	m.grants[name] = nil
	return nil
}

func (m *fakeManager) RemoveRepository(name string) error {
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[name]; !ok {
		return errors.New("repository not found")
	}
	delete(m.grants, name)
	return nil
}

func (m *fakeManager) GetRepository(name string) (repository.Repository, error) {
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[name]; !ok {
		return repository.Repository{}, errors.New("repository not found")
	}
	return repository.Repository{
		Name:         name,
		ReadOnlyURL:  fmt.Sprintf("git://%s/%s.git", ServerHost, name),
		ReadWriteURL: fmt.Sprintf("git@%s:%s.git", ServerHost, name),
	}, nil
}

func (m *fakeManager) GrantAccess(repository, user string) error {
	m.keysLock.Lock()
	_, ok := m.keys[user]
	m.keysLock.Unlock()
	if !ok {
		return errors.New("user not found")
	}
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	grants, ok := m.grants[repository]
	if !ok {
		return errors.New("repository not found")
	}
	var found bool
	for _, granted := range grants {
		if granted == user {
			found = true
			break
		}
	}
	if !found {
		grants = append(grants, user)
		m.grants[repository] = grants
	}
	return nil
}

func (m *fakeManager) RevokeAccess(repository, user string) error {
	m.keysLock.Lock()
	_, ok := m.keys[user]
	m.keysLock.Unlock()
	if !ok {
		return errors.New("user not found")
	}
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	grants, ok := m.grants[repository]
	if !ok {
		return errors.New("repository not found")
	}
	index := -1
	for i, granted := range grants {
		if granted == user {
			index = i
			break
		}
	}
	if index > -1 {
		last := len(grants) - 1
		grants[index] = grants[last]
		m.grants[repository] = grants[:last]
	}
	return nil
}

func (m *fakeManager) AddKey(username string, key repository.Key) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	keys, ok := m.keys[username]
	if !ok {
		return errors.New("user not found")
	}
	if _, ok := keys[key.Name]; ok {
		return errors.New("user already have a key with this name")
	}
	keys[key.Name] = key.Body
	m.keys[username] = keys
	return nil
}

func (m *fakeManager) RemoveKey(username string, key repository.Key) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	keys, ok := m.keys[username]
	if !ok {
		return errors.New("user not found")
	}
	if _, ok := keys[key.Name]; !ok {
		return errors.New("key not found")
	}
	delete(keys, key.Name)
	m.keys[username] = keys
	return nil
}

func (m *fakeManager) ListKeys(username string) ([]repository.Key, error) {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	keys, ok := m.keys[username]
	if !ok {
		return nil, errors.New("user not found")
	}
	result := make([]repository.Key, 0, len(keys))
	for name, body := range keys {
		result = append(result, repository.Key{Name: name, Body: body})
	}
	return result, nil
}

func (m *fakeManager) Diff(repositoryName, from, to string) (string, error) {
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[repositoryName]; !ok {
		return "", errors.New("repository not found")
	}
	return "", nil
}

// Reset resets the internal state of the fake manager.
func Reset() {
	manager.grantsLock.Lock()
	defer manager.grantsLock.Unlock()
	manager.keysLock.Lock()
	defer manager.keysLock.Unlock()
	manager.grants = make(map[string][]string)
	manager.keys = make(map[string]map[string]string)
}

// Users returns the list of users currently registered in the fake manager.
func Users() []string {
	manager.keysLock.Lock()
	defer manager.keysLock.Unlock()
	users := make([]string, 0, len(manager.keys))
	for user := range manager.keys {
		users = append(users, user)
	}
	return users
}

// Granted returns the list of users with access granted to the given
// repository name, failing if the given repository isn't registered.
func Granted(repository string) ([]string, error) {
	manager.grantsLock.Lock()
	defer manager.grantsLock.Unlock()
	if grants, ok := manager.grants[repository]; ok {
		return grants, nil
	}
	return nil, errors.New("repository not found")
}
