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
	"errors"
	"net/http"

	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/repository"
)

func init() {
	repository.Register("gandalf", gandalfManager{})
	hc.AddChecker("Gandalf", healthCheck)
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

type gandalfManager struct{}

func (gandalfManager) client() (*gandalf.Client, error) {
	url, err := config.GetString(endpointConfig)
	if err != nil {
		return nil, err
	}
	client := gandalf.Client{Endpoint: url}
	return &client, nil
}

func Sync() error {
	var m gandalfManager
	users, err := auth.ListUsers()
	if err != nil {
		return err
	}
	for _, user := range users {
		err = m.CreateUser(user.Email)
		if err != nil && err != repository.ErrUserAlreadyExists {
			return err
		}
	}
	apps, err := app.List(nil)
	if err != nil {
		return err
	}
	for _, app := range apps {
		var users []string
		for _, team := range app.GetTeams() {
			users = append(users, team.Users...)
		}
		err = m.CreateRepository(app.Name, users)
		if err != nil && err != repository.ErrRepositoryAlreadExists {
			return err
		}
		for _, user := range users {
			m.GrantAccess(app.Name, user)
		}
	}
	return nil
}

func (m gandalfManager) CreateUser(username string) error {
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

func (m gandalfManager) RemoveUser(username string) error {
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

func (m gandalfManager) CreateRepository(name string, users []string) error {
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

func (m gandalfManager) RemoveRepository(name string) error {
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

func (m gandalfManager) GetRepository(name string) (repository.Repository, error) {
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
	return repository.Repository{
		Name:         repo.Name,
		ReadOnlyURL:  repo.GitURL,
		ReadWriteURL: repo.SshURL,
	}, nil
}

func (m gandalfManager) GrantAccess(repository, user string) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	return client.GrantAccess([]string{repository}, []string{user})
}

func (m gandalfManager) RevokeAccess(repository, user string) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	return client.RevokeAccess([]string{repository}, []string{user})
}

func (m gandalfManager) AddKey(username string, key repository.Key) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	keyMap := map[string]string{key.Name: key.Body}
	return client.AddKey(username, keyMap)
}

func (m gandalfManager) RemoveKey(username string, key repository.Key) error {
	client, err := m.client()
	if err != nil {
		return err
	}
	return client.RemoveKey(username, key.Name)
}

func (m gandalfManager) ListKeys(username string) ([]repository.Key, error) {
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

func (m gandalfManager) Diff(name, from, to string) (string, error) {
	client, err := m.client()
	if err != nil {
		return "", err
	}
	return client.GetDiff(name, from, to)
}
