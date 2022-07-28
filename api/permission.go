// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

// title: role create
// path: /roles
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   201: Role created
//   400: Invalid data
//   401: Unauthorized
//   409: Role already exists
func addRole(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermRoleCreate) {
		return permission.ErrUnauthorized
	}
	roleName := InputValue(r, "name")
	if roleName == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: permTypes.ErrInvalidRoleName.Error(),
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	_, err = permission.NewRole(roleName, InputValue(r, "context"), InputValue(r, "description"))
	if err == permTypes.ErrInvalidRoleName {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if err == permTypes.ErrRoleAlreadyExists {
		return &errors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

// title: remove role
// path: /roles/{name}
// method: DELETE
// responses:
//   200: Role removed
//   401: Unauthorized
//   404: Role not found
//   412: Role with users
func removeRole(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermRoleDelete) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	usersWithRole, err := auth.ListUsersWithRole(roleName)
	if err != nil {
		return err
	}
	if len(usersWithRole) != 0 {
		return &errors.HTTP{Code: http.StatusPreconditionFailed, Message: permTypes.ErrRemoveRoleWithUsers.Error()}
	}
	err = permission.DestroyRole(roleName)
	if err == permTypes.ErrRoleNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return err
}

// title: role list
// path: /roles
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
func listRoles(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !(permission.Check(t, permission.PermRoleUpdate) ||
		permission.Check(t, permission.PermRoleUpdateAssign) ||
		permission.Check(t, permission.PermRoleUpdateDissociate) ||
		permission.Check(t, permission.PermRoleCreate) ||
		permission.Check(t, permission.PermRoleDelete)) {
		return permission.ErrUnauthorized
	}
	roles, err := permission.ListRoles()
	if err != nil {
		return err
	}
	b, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(b)
	return err
}

// title: role info
// path: /roles/{name}
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Role not found
func roleInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !(permission.Check(t, permission.PermRoleUpdate) ||
		permission.Check(t, permission.PermRoleUpdateAssign) ||
		permission.Check(t, permission.PermRoleUpdateDissociate) ||
		permission.Check(t, permission.PermRoleCreate) ||
		permission.Check(t, permission.PermRoleDelete)) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	role, err := permission.FindRole(roleName)
	if err == permTypes.ErrRoleNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
	if err != nil {
		return err
	}
	b, err := json.Marshal(role)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(b)
	return err
}

// title: add permissions
// path: /roles/{name}/permissions
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   409: Permission not allowed
func addPermissions(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermRoleUpdatePermissionAdd) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdatePermissionAdd,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	role, err := permission.FindRole(roleName)
	if err != nil {
		return err
	}

	permissions, _ := InputValues(r, "permission")
	err = role.AddPermissions(permissions...)

	if err == permTypes.ErrInvalidPermissionName {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if perr, ok := err.(*permTypes.ErrPermissionNotFound); ok {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: perr.Error(),
		}
	}
	if perr, ok := err.(*permTypes.ErrPermissionNotAllowed); ok {
		return &errors.HTTP{
			Code:    http.StatusConflict,
			Message: perr.Error(),
		}
	}
	return err
}

// title: remove permission
// path: /roles/{name}/permissions/{permission}
// method: DELETE
// responses:
//   200: Permission removed
//   401: Unauthorized
//   404: Not found
func removePermissions(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermRoleUpdatePermissionRemove) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdatePermissionRemove,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	permName := r.URL.Query().Get(":permission")
	role, err := permission.FindRole(roleName)
	if err != nil {
		if err == permTypes.ErrRoleNotFound {
			return &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}

	return role.RemovePermissions(permName)
}

func validateContexType(role permission.Role, contextType string) error {
	if string(role.ContextType) != contextType {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("wrong context-type, role %s has context-type: %s", role.Name, role.ContextType),
		}
	}
	return nil
}

func getRoleReturnNotFound(roleName string) (permission.Role, error) {
	role, err := permission.FindRole(roleName)
	if err != nil {
		if err == permTypes.ErrRoleNotFound {
			return permission.Role{}, &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return permission.Role{}, err
	}
	return role, nil
}

func canUseRole(t auth.Token, role permission.Role, contextValue string) error {
	userPerms, err := t.Permissions()
	if err != nil {
		return err
	}
	perms := role.PermissionsFor(contextValue)
	for _, p := range perms {
		if !permission.CheckFromPermList(userPerms, p.Scheme, p.Context) {
			return &errors.HTTP{
				Code:    http.StatusForbidden,
				Message: fmt.Sprintf("User not authorized to use permission %s", p.String()),
			}
		}
	}
	return nil
}

// title: assign role to user
// path: /roles/{name}/user
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Role not found
func assignRole(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermRoleUpdateAssign) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdateAssign,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	email := InputValue(r, "email")
	contextValue := InputValue(r, "context")
	contextType := InputValue(r, "contextType")
	if contextType == "" {
		contextType = "global"
	}
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return err
	}

	role, err := getRoleReturnNotFound(roleName)
	if err != nil {
		return err
	}
	if err = validateContexType(role, contextType); err != nil {
		return err
	}

	err = canUseRole(t, role, contextValue)
	if err != nil {
		return err
	}

	return user.AddRole(roleName, contextValue)
}

// title: dissociate role from user
// path: /roles/{name}/user/{email}
// method: DELETE
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Role not found
func dissociateRole(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermRoleUpdateDissociate) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdateDissociate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	email := r.URL.Query().Get(":email")
	contextValue := r.URL.Query().Get("context")
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return err
	}

	role, err := getRoleReturnNotFound(roleName)
	if err != nil {
		return err
	}
	err = canUseRole(t, role, contextValue)
	if err != nil {
		return err
	}

	return user.RemoveRole(roleName, contextValue)
}

type permissionSchemeData struct {
	Name     string
	Contexts []string
}

// title: list permissions
// path: /permissions
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
func listPermissions(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleUpdate) {
		return permission.ErrUnauthorized
	}
	lst := permission.PermissionRegistry.Permissions()
	sort.Sort(lst)
	permList := make([]permissionSchemeData, len(lst))
	for i, perm := range lst {
		contexts := perm.AllowedContexts()
		contextNames := make([]string, len(contexts))
		for j, ctx := range contexts {
			contextNames[j] = string(ctx)
		}
		permList[i] = permissionSchemeData{
			Name:     perm.FullName(),
			Contexts: contextNames,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(permList)
}

// title: add default role
// path: /role/default
// method: POST
// consme: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
func addDefaultRole(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermRoleDefaultCreate) {
		return permission.ErrUnauthorized
	}
	rolesMap := map[string][]string{}
	for evtName := range permTypes.RoleEventMap {
		roles, _ := InputValues(r, evtName)
		for _, roleName := range roles {
			rolesMap[roleName] = append(rolesMap[roleName], evtName)
		}
	}
	for roleName, evts := range rolesMap {
		evt, err := event.New(&event.Opts{
			Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
			Kind:       permission.PermRoleDefaultCreate,
			Owner:      t,
			CustomData: event.FormToCustomData(InputFields(r)),
			Allowed:    event.Allowed(permission.PermRoleReadEvents),
		})
		if err != nil {
			return err
		}
		defer func() { evt.Done(err) }()
		role, err := permission.FindRole(roleName)
		if err != nil {
			if err == permTypes.ErrRoleNotFound {
				return &errors.HTTP{
					Code:    http.StatusBadRequest,
					Message: err.Error(),
				}
			}
			return err
		}
		for _, evtName := range evts {
			err = role.AddEvent(evtName)
			if err != nil {
				if _, ok := err.(permTypes.ErrRoleEventWrongContext); ok {
					return &errors.HTTP{
						Code:    http.StatusBadRequest,
						Message: err.Error(),
					}
				}
				return err
			}
		}
	}
	return nil
}

// title: remove default role
// path: /role/default
// method: DELETE
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
func removeDefaultRole(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermRoleDefaultDelete) {
		return permission.ErrUnauthorized
	}

	rolesMap := map[string][]string{}
	for evtName := range permTypes.RoleEventMap {
		roles, _ := InputValues(r, evtName)
		for _, roleName := range roles {
			rolesMap[roleName] = append(rolesMap[roleName], evtName)
		}
	}
	for roleName, evts := range rolesMap {
		evt, err := event.New(&event.Opts{
			Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
			Kind:       permission.PermRoleDefaultDelete,
			Owner:      t,
			CustomData: event.FormToCustomData(InputFields(r)),
			Allowed:    event.Allowed(permission.PermRoleReadEvents),
		})
		if err != nil {
			return err
		}
		defer func() { evt.Done(err) }()
		role, err := permission.FindRole(roleName)
		if err != nil {
			if err == permTypes.ErrRoleNotFound {
				return &errors.HTTP{
					Code:    http.StatusBadRequest,
					Message: err.Error(),
				}
			}
			return err
		}
		for _, evtName := range evts {
			err = role.RemoveEvent(evtName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// title: list default roles
// path: /role/default
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
func listDefaultRoles(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleDefaultCreate) &&
		!permission.Check(t, permission.PermRoleDefaultDelete) {
		return permission.ErrUnauthorized
	}
	roles, err := permission.ListRolesWithEvents()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(roles)
}

// title: updates a role
// path: /roles
// method: PUT
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
func roleUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	roleName := InputValue(r, "name")
	newName := InputValue(r, "newName")
	contextType := InputValue(r, "contextType")
	description := InputValue(r, "description")
	var wantedPerms []*permission.PermissionScheme
	if newName != "" {
		wantedPerms = append(wantedPerms, permission.PermRoleUpdateName)
	}
	if contextType != "" {
		wantedPerms = append(wantedPerms, permission.PermRoleUpdateContextType)
	}
	if description != "" {
		wantedPerms = append(wantedPerms, permission.PermRoleUpdateDescription)
	}
	if len(wantedPerms) == 0 {
		msg := "Neither the description, context or new name were set. You must define at least one."
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	for _, perm := range wantedPerms {
		if !permission.Check(t, perm) {
			return permission.ErrUnauthorized
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleUpdate),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = auth.UpdateRoleFromAllUsers(roleName, newName, contextType, description)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	return nil
}

// title: assign role to token
// path: /roles/{name}/token
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Role or team token not found
func assignRoleToToken(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	if !permission.Check(t, permission.PermRoleUpdateAssign) {
		return permission.ErrUnauthorized
	}
	contextType := InputValue(r, "contextType")
	if contextType == "" {
		contextType = "global"
	}
	tokenID := InputValue(r, "token_id")
	contextValue := InputValue(r, "context")
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdateAssign,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	role, err := getRoleReturnNotFound(roleName)
	if err != nil {
		return err
	}
	if err = validateContexType(role, contextType); err != nil {
		return err
	}

	err = canUseRole(t, role, contextValue)
	if err != nil {
		return err
	}
	err = servicemanager.TeamToken.AddRole(ctx, tokenID, roleName, contextValue)
	if err == authTypes.ErrTeamTokenNotFound {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}
	return err
}

// title: dissociate role from token
// path: /roles/{name}/token/{token_id}
// method: DELETE
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Role or team token not found
func dissociateRoleFromToken(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	if !permission.Check(t, permission.PermRoleUpdateDissociate) {
		return permission.ErrUnauthorized
	}
	tokenID := r.URL.Query().Get(":token_id")
	contextValue := InputValue(r, "context")
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdateDissociate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	role, err := getRoleReturnNotFound(roleName)
	if err != nil {
		return err
	}
	err = canUseRole(t, role, contextValue)
	if err != nil {
		return err
	}
	err = servicemanager.TeamToken.RemoveRole(ctx, tokenID, roleName, contextValue)
	if err == authTypes.ErrTeamTokenNotFound {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}
	return err
}

// title: assign role to group
// path: /roles/{name}/group
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Role not found
func assignRoleToGroup(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleUpdateAssign) {
		return permission.ErrUnauthorized
	}
	groupName := InputValue(r, "group_name")
	contextValue := InputValue(r, "context")
	contextType := InputValue(r, "contextType")
	if contextType == "" {
		contextType = "global"
	}
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdateAssign,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	role, err := getRoleReturnNotFound(roleName)
	if err != nil {
		return err
	}
	if err = validateContexType(role, contextType); err != nil {
		return err
	}

	err = canUseRole(t, role, contextValue)
	if err != nil {
		return err
	}
	return servicemanager.AuthGroup.AddRole(groupName, roleName, contextValue)
}

// title: dissociate role from group
// path: /roles/{name}/group/{group_name}
// method: DELETE
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Role not found
func dissociateRoleFromGroup(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleUpdateDissociate) {
		return permission.ErrUnauthorized
	}
	groupName := r.URL.Query().Get(":group_name")
	contextValue := InputValue(r, "context")
	roleName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: roleName},
		Kind:       permission.PermRoleUpdateDissociate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRoleReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	role, err := getRoleReturnNotFound(roleName)
	if err != nil {
		return err
	}

	err = canUseRole(t, role, contextValue)
	if err != nil {
		return err
	}
	return servicemanager.AuthGroup.RemoveRole(groupName, roleName, contextValue)
}
