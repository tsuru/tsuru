// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"errors"
	"fmt"
)

var (
	ErrRoleNotFound          = errors.New("role not found")
	ErrRoleAlreadyExists     = errors.New("role already exists")
	ErrRoleEventNotFound     = errors.New("role event not found")
	ErrInvalidRoleName       = errors.New("invalid role name")
	ErrInvalidPermissionName = errors.New("invalid permission name")
	ErrRemoveRoleWithUsers   = errors.New("role has users assigned. you must dissociate them before remove the role.")

	RoleEventUserCreate = &RoleEvent{
		Name:        "user-create",
		Context:     CtxGlobal,
		Description: "role added to user when user is created",
	}
	RoleEventTeamCreate = &RoleEvent{
		Name:        "team-create",
		Context:     CtxTeam,
		Description: "role added to user when a new team is created",
	}

	RoleEventMap = map[string]*RoleEvent{
		RoleEventUserCreate.Name: RoleEventUserCreate,
		RoleEventTeamCreate.Name: RoleEventTeamCreate,
	}
)

type RoleEvent struct {
	Name        string
	Context     ContextType
	Description string
}

func (e *RoleEvent) String() string {
	return e.Name
}

type ErrRoleEventWrongContext struct {
	Expected string
	Role     string
}

func (e ErrRoleEventWrongContext) Error() string {
	return fmt.Sprintf("wrong context type for role event, expected %q role has %q", e.Expected, e.Role)
}

type ErrPermissionNotFound struct {
	Permission string
}

func (e ErrPermissionNotFound) Error() string {
	return fmt.Sprintf("permission named %q not found", e.Permission)
}

type ErrPermissionNotAllowed struct {
	Permission  string
	ContextType ContextType
}

func (e ErrPermissionNotAllowed) Error() string {
	return fmt.Sprintf("permission %q not allowed with context of type %q", e.Permission, e.ContextType)
}
