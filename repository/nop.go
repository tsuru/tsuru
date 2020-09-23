// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import "context"

func init() {
	Register("nop", nopManager{})
}

type nopManager struct{}

func (nopManager) CreateUser(ctx context.Context, username string) error {
	return nil
}

func (nopManager) RemoveUser(ctx context.Context, username string) error {
	return nil
}

func (nopManager) GrantAccess(ctx context.Context, repository, user string) error {
	return nil
}

func (nopManager) RevokeAccess(ctx context.Context, repository, user string) error {
	return nil
}

func (nopManager) CreateRepository(ctx context.Context, name string, users []string) error {
	return nil
}

func (nopManager) RemoveRepository(ctx context.Context, name string) error {
	return nil
}

func (nopManager) GetRepository(ctx context.Context, name string) (Repository, error) {
	return Repository{}, nil
}

func (nopManager) Diff(ctx context.Context, repositoryName, from, to string) (string, error) {
	return "", nil
}

func (nopManager) CommitMessages(ctx context.Context, repository, ref string, limit int) ([]string, error) {
	return nil, nil
}
