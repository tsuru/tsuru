// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

var (
	_ authTypes.GroupService = &groupService{}

	errGroupNameEmpty = errors.New("group name cannot be empty")
)

func GroupService() (authTypes.GroupService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &groupService{
		storage: dbDriver.AuthGroupStorage,
	}, nil
}

type groupService struct {
	storage authTypes.GroupStorage
}

func (s *groupService) List(ctx context.Context, filter []string) ([]authTypes.Group, error) {
	return s.storage.List(ctx, filter)
}

func (s *groupService) AddRole(name, roleName, contextValue string) error {
	if name == "" {
		return errGroupNameEmpty
	}
	_, err := permission.FindRole(roleName)
	if err != nil {
		return err
	}
	return s.storage.AddRole(name, roleName, contextValue)
}

func (s *groupService) RemoveRole(name, roleName, contextValue string) error {
	if name == "" {
		return errGroupNameEmpty
	}
	return s.storage.RemoveRole(name, roleName, contextValue)
}
