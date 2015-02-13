// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

func init() {
	Register("nop", nopManager{})
}

type nopManager struct{}

func (nopManager) CreateUser(username string) error {
	return nil
}

func (nopManager) RemoveUser(username string) error {
	return nil
}

func (nopManager) GrantAccess(repository, user string) error {
	return nil
}

func (nopManager) RevokeAccess(repository, user string) error {
	return nil
}

func (nopManager) CreateRepository(name string) error {
	return nil
}

func (nopManager) RemoveRepository(name string) error {
	return nil
}

func (nopManager) GetRepository(name string) (Repository, error) {
	return Repository{}, nil
}

func (nopManager) Diff(repositoryName, from, to string) (string, error) {
	return "", nil
}
