// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

type Group struct {
	Name  string         `json:"name"`
	Roles []RoleInstance `json:"roles,omitempty"`
}

type GroupStorage interface {
	GroupService
}

type GroupService interface {
	List(filter []string) ([]Group, error)
	AddRole(name, roleName, contextValue string) error
	RemoveRole(name, roleName, contextValue string) error
}
