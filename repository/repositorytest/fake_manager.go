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
	"context"
	"crypto/rand"
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

// Diff is the diff returned by the Diff method.
const Diff = "fake-diff"

var manager = fakeManager{grants: make(map[string][]string), keys: make(map[string]map[string]string)}

var (
	_ repository.RepositoryManager    = &fakeManager{}
	_ repository.KeyRepositoryManager = &fakeManager{}
)

type fakeManager struct {
	grants     map[string][]string
	grantsLock sync.Mutex
	keys       map[string]map[string]string
	keysLock   sync.Mutex
}

func (m *fakeManager) CreateUser(ctx context.Context, username string) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	if _, ok := m.keys[username]; ok {
		return repository.ErrUserAlreadyExists
	}
	m.keys[username] = make(map[string]string)
	return nil
}

func (m *fakeManager) RemoveUser(ctx context.Context, username string) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	if _, ok := m.keys[username]; !ok {
		return repository.ErrUserNotFound
	}
	delete(m.keys, username)
	return nil
}

func (m *fakeManager) CreateRepository(ctx context.Context, name string, users []string) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	for _, user := range users {
		if _, ok := m.keys[user]; !ok {
			return repository.ErrUserNotFound
		}
	}
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[name]; ok {
		return repository.ErrRepositoryAlreadExists
	}
	m.grants[name] = users
	return nil
}

func (m *fakeManager) RemoveRepository(ctx context.Context, name string) error {
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[name]; !ok {
		return repository.ErrRepositoryNotFound
	}
	delete(m.grants, name)
	return nil
}

func (m *fakeManager) GetRepository(ctx context.Context, name string) (repository.Repository, error) {
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[name]; !ok {
		return repository.Repository{}, repository.ErrRepositoryNotFound
	}
	return repository.Repository{
		Name:         name,
		ReadWriteURL: fmt.Sprintf("git@%s:%s.git", ServerHost, name),
	}, nil
}

func (m *fakeManager) GrantAccess(ctx context.Context, repo, user string) error {
	m.keysLock.Lock()
	_, ok := m.keys[user]
	m.keysLock.Unlock()
	if !ok {
		return repository.ErrUserNotFound
	}
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	grants, ok := m.grants[repo]
	if !ok {
		return repository.ErrRepositoryNotFound
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
		m.grants[repo] = grants
	}
	return nil
}

func (m *fakeManager) RevokeAccess(ctx context.Context, repo, user string) error {
	m.keysLock.Lock()
	_, ok := m.keys[user]
	m.keysLock.Unlock()
	if !ok {
		return repository.ErrUserNotFound
	}
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	grants, ok := m.grants[repo]
	if !ok {
		return repository.ErrRepositoryNotFound
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
		m.grants[repo] = grants[:last]
	}
	return nil
}

func (m *fakeManager) AddKey(ctx context.Context, username string, key repository.Key) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	keys, ok := m.keys[username]
	if !ok {
		return repository.ErrUserNotFound
	}
	if key.Name == "" {
		var p [32]byte
		rand.Read(p[:])
		key.Name = string(p[:])
	}
	if _, ok := keys[key.Name]; ok {
		return repository.ErrKeyAlreadyExists
	}
	keys[key.Name] = key.Body
	m.keys[username] = keys
	return nil
}

func (m *fakeManager) UpdateKey(ctx context.Context, username string, key repository.Key) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	keys, ok := m.keys[username]
	if !ok {
		return repository.ErrUserNotFound
	}
	if key.Name == "" {
		var p [32]byte
		rand.Read(p[:])
		key.Name = string(p[:])
	}
	if _, ok := keys[key.Name]; !ok {
		return repository.ErrKeyNotFound
	}
	keys[key.Name] = key.Body
	m.keys[username] = keys
	return nil
}

func (m *fakeManager) RemoveKey(ctx context.Context, username string, key repository.Key) error {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	keys, ok := m.keys[username]
	if !ok {
		return repository.ErrUserNotFound
	}
	if _, ok := keys[key.Name]; !ok {
		return repository.ErrKeyNotFound
	}
	delete(keys, key.Name)
	m.keys[username] = keys
	return nil
}

func (m *fakeManager) ListKeys(ctx context.Context, username string) ([]repository.Key, error) {
	m.keysLock.Lock()
	defer m.keysLock.Unlock()
	keys, ok := m.keys[username]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	result := make([]repository.Key, 0, len(keys))
	for name, body := range keys {
		result = append(result, repository.Key{Name: name, Body: body})
	}
	return result, nil
}

func (m *fakeManager) Diff(ctx context.Context, repositoryName, from, to string) (string, error) {
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[repositoryName]; !ok {
		return "", repository.ErrRepositoryNotFound
	}
	return Diff, nil
}

func (m *fakeManager) CommitMessages(ctx context.Context, repositoryName, ref string, limit int) ([]string, error) {
	m.grantsLock.Lock()
	defer m.grantsLock.Unlock()
	if _, ok := m.grants[repositoryName]; !ok {
		return nil, repository.ErrRepositoryNotFound
	}
	return []string{"msg1", "msg2"}, nil
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
func Granted(repo string) ([]string, error) {
	manager.grantsLock.Lock()
	defer manager.grantsLock.Unlock()
	if grants, ok := manager.grants[repo]; ok {
		return grants, nil
	}
	return nil, repository.ErrRepositoryNotFound
}
