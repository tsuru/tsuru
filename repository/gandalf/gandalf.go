// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gandalf provides an implementation of the RepositoryManager, that
// uses Gandalf (https://github.com/tsuru/gandalf). This package doesn't expose
// any public types, in order to use it, users need to import the package and
// then configure tsuru to use the "gandalf" repo-manager.
//
//     import _ "github.com/tsuru/tsuru/repository/gandalf"
package gandalf

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	gandalf "github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/repository"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

func init() {
	repository.Register("gandalf", newManager())
	hc.AddChecker("Gandalf", healthCheck)
}

func newManager() *gandalfManager {
	return &gandalfManager{
		repoCache: make(map[string]repository.Repository),
	}
}

const endpointConfig = "git:api-server"

func healthCheck() error {
	serverURL, _ := config.GetString(endpointConfig)
	if serverURL == "" {
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

type gandalfManager struct {
	mu        sync.RWMutex
	repoCache map[string]repository.Repository
}

var (
	_ repository.RepositoryManager    = &gandalfManager{}
	_ repository.KeyRepositoryManager = &gandalfManager{}
)

func (*gandalfManager) client() (*gandalf.Client, error) {
	url, err := config.GetString(endpointConfig)
	if err != nil {
		return nil, err
	}
	client := gandalf.Client{Endpoint: url}
	return &client, nil
}

func Sync(ctx context.Context, w io.Writer) error {
	var m gandalfManager
	users, err := auth.ListUsers()
	if err != nil {
		return err
	}
	for _, user := range users {
		fmt.Fprintf(w, "Syncing user %q... ", user.Email)
		err = m.CreateUser(ctx, user.Email)
		switch err {
		case repository.ErrUserAlreadyExists:
			fmt.Fprintln(w, "already present in Gandalf")
		case nil:
			fmt.Fprintln(w, "OK")
		default:
			return err
		}
	}
	apps, err := app.List(context.TODO(), nil)
	if err != nil {
		return err
	}
	for _, app := range apps {
		allowedPerms := []permission.Permission{
			{
				Scheme:  permission.PermAppDeploy,
				Context: permission.Context(permTypes.CtxGlobal, ""),
			},
			{
				Scheme:  permission.PermAppDeploy,
				Context: permission.Context(permTypes.CtxPool, app.GetPool()),
			},
		}
		for _, name := range app.GetTeamsName() {
			allowedPerms = append(allowedPerms, permission.Permission{
				Scheme:  permission.PermAppDeploy,
				Context: permission.Context(permTypes.CtxTeam, name),
			})
		}
		users, err := auth.ListUsersWithPermissions(allowedPerms...)
		if err != nil {
			return err
		}
		userNames := make([]string, len(users))
		for i := range users {
			userNames[i] = users[i].Email
		}
		fmt.Fprintf(w, "Syncing app %q... ", app.GetName())
		err = m.CreateRepository(ctx, app.GetName(), userNames)
		switch err {
		case repository.ErrRepositoryAlreadExists:
			fmt.Fprintln(w, "already present in Gandalf")
		case nil:
			fmt.Fprintln(w, "OK")
		default:
			return err
		}
		for _, user := range userNames {
			m.GrantAccess(ctx, app.GetName(), user)
		}
	}
	return nil
}

func (m *gandalfManager) CreateUser(ctx context.Context, username string) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	_, err = client.NewUser(username, nil)
	if e, ok := err.(*gandalf.HTTPError); ok && e.Code == http.StatusConflict {
		return repository.ErrUserAlreadyExists
	}
	return err
}

func (m *gandalfManager) RemoveUser(ctx context.Context, username string) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	err = client.RemoveUser(username)
	if e, ok := err.(*gandalf.HTTPError); ok && e.Code == http.StatusNotFound {
		return repository.ErrUserNotFound
	}
	return err
}

func (m *gandalfManager) CreateRepository(ctx context.Context, name string, users []string) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	_, err = client.NewRepository(name, users, true)
	if e, ok := err.(*gandalf.HTTPError); ok && e.Code == http.StatusConflict {
		return repository.ErrRepositoryAlreadExists
	}
	return err
}

func (m *gandalfManager) RemoveRepository(ctx context.Context, name string) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	err = client.RemoveRepository(name)
	if e, ok := err.(*gandalf.HTTPError); ok && e.Code == http.StatusNotFound {
		return repository.ErrRepositoryNotFound
	}
	return err
}

func (m *gandalfManager) getRepositoryCached(name string) (repository.Repository, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.repoCache[name]
	return r, ok
}

func (m *gandalfManager) GetRepository(ctx context.Context, name string) (repository.Repository, error) {
	r, ok := m.getRepositoryCached(name)
	if ok {
		return r, nil
	}
	client, err := m.client()
	if err != nil {
		return repository.Repository{}, err
	}
	repo, err := client.GetRepository(name)
	if e, ok := err.(*gandalf.HTTPError); ok && e.Code == http.StatusNotFound {
		return repository.Repository{}, repository.ErrRepositoryNotFound
	}
	if err != nil {
		return repository.Repository{}, err
	}
	r = repository.Repository{
		Name:         repo.Name,
		ReadWriteURL: repo.SshURL,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repoCache[name] = r
	return r, nil
}

func (m *gandalfManager) GrantAccess(ctx context.Context, repository, user string) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	return client.GrantAccess([]string{repository}, []string{user})
}

func (m *gandalfManager) RevokeAccess(ctx context.Context, repository, user string) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	return client.RevokeAccess([]string{repository}, []string{user})
}

func (m *gandalfManager) AddKey(ctx context.Context, username string, key repository.Key) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	keyMap := map[string]string{key.Name: key.Body}
	err = client.AddKey(username, keyMap)
	if err != nil {
		if e, ok := err.(*gandalf.HTTPError); ok && e.Code == http.StatusConflict {
			return repository.ErrKeyAlreadyExists
		}
		return err
	}
	return nil
}

func (m *gandalfManager) UpdateKey(ctx context.Context, username string, key repository.Key) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	return m.handleKeyOrUserError(client.UpdateKey(username, key.Name, key.Body))
}

func (m *gandalfManager) RemoveKey(ctx context.Context, username string, key repository.Key) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	return m.handleKeyOrUserError(client.RemoveKey(username, key.Name))
}

func (*gandalfManager) handleKeyOrUserError(err error) error {
	if err != nil {
		if e, ok := err.(*gandalf.HTTPError); ok {
			if e.Code == http.StatusNotFound {
				switch e.Reason {
				case "user not found\n":
					return repository.ErrUserNotFound
				case "Key not found\n":
					return repository.ErrKeyNotFound
				}
			}
		}
		return err
	}
	return nil
}

func (m *gandalfManager) ListKeys(ctx context.Context, username string) ([]repository.Key, error) {
	client, err := m.client()
	if err != nil {
		return nil, err
	}
	keyMap, err := client.ListKeys(username)
	if err != nil {
		return nil, err
	}
	keys := make([]repository.Key, 0, len(keyMap))
	for name, body := range keyMap {
		keys = append(keys, repository.Key{Name: name, Body: body})
	}
	return keys, nil
}

func (m *gandalfManager) Diff(ctx context.Context, name, from, to string) (string, error) {
	client, err := m.client()
	if err != nil {
		return "", err
	}
	return client.GetDiff(name, from, to)
}

func (m *gandalfManager) CommitMessages(ctx context.Context, repository, ref string, limit int) ([]string, error) {
	client, err := m.client()
	if err != nil {
		return nil, err
	}
	log, err := client.GetLog(repository, ref, "", limit)
	if err != nil {
		return nil, err
	}
	msgs := make([]string, len(log.Commits))
	for i := range log.Commits {
		msgs[i] = log.Commits[i].Subject
	}
	return msgs, nil
}
