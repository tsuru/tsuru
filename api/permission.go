// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/repository"
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
func addRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleCreate) {
		return permission.ErrUnauthorized
	}
	_, err := permission.NewRole(r.FormValue("name"), r.FormValue("context"), r.FormValue("description"))
	if err == permission.ErrInvalidRoleName {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if err == permission.ErrRoleAlreadyExists {
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
func removeRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleDelete) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	err := auth.RemoveRoleFromAllUsers(roleName)
	if err != nil {
		return err
	}
	err = permission.DestroyRole(roleName)
	if err == permission.ErrRoleNotFound {
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
	if err == permission.ErrRoleNotFound {
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

func deployableApps(u *auth.User, rolesCache map[string]*permission.Role) ([]string, error) {
	var perms []permission.Permission
	for _, roleData := range u.Roles {
		role := rolesCache[roleData.Name]
		if role == nil {
			foundRole, err := permission.FindRole(roleData.Name)
			if err != nil {
				return nil, err
			}
			role = &foundRole
			rolesCache[roleData.Name] = role
		}
		perms = append(perms, role.PermissionsFor(roleData.ContextValue)...)
	}
	contexts := permission.ContextsFromListForPermission(perms, permission.PermAppDeploy)
	if len(contexts) == 0 {
		return nil, nil
	}
	filter := appFilterByContext(contexts, nil)
	apps, err := app.List(filter)
	if err != nil {
		return nil, err
	}
	appNames := make([]string, len(apps))
	for i := range apps {
		appNames[i] = apps[i].GetName()
	}
	return appNames, nil
}

func syncRepositoryApps(user *auth.User, beforeApps []string, roleCache map[string]*permission.Role) error {
	err := user.Reload()
	if err != nil {
		return err
	}
	afterApps, err := deployableApps(user, roleCache)
	if err != nil {
		return err
	}
	afterMap := map[string]struct{}{}
	for _, a := range afterApps {
		afterMap[a] = struct{}{}
	}
	manager := repository.Manager()
	for _, a := range beforeApps {
		var err error
		if _, ok := afterMap[a]; !ok {
			err = manager.RevokeAccess(a, user.Email)
		}
		if err != nil {
			log.Errorf("error revoking gandalf access for app %s, user %s: %s", a, user.Email, err)
		}
	}
	for _, a := range afterApps {
		err := manager.GrantAccess(a, user.Email)
		if err != nil {
			log.Errorf("error granting gandalf access for app %s, user %s: %s", a, user.Email, err)
		}
	}
	return nil

}

func runWithPermSync(users []auth.User, callback func() error) error {
	usersMap := make(map[*auth.User][]string)
	roleCache := make(map[string]*permission.Role)
	for i := range users {
		u := &users[i]
		apps, err := deployableApps(u, roleCache)
		if err != nil {
			return err
		}
		usersMap[u] = apps
	}
	err := callback()
	if err != nil {
		return err
	}
	roleCache = make(map[string]*permission.Role)
	for u, apps := range usersMap {
		err = syncRepositoryApps(u, apps, roleCache)
		if err != nil {
			log.Errorf("unable to sync gandalf repositories updating permissions: %s", err)
		}
	}
	return nil
}

func addPermissions(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleUpdate) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	role, err := permission.FindRole(roleName)
	if err != nil {
		return err
	}
	err = r.ParseForm()
	if err != nil {
		return err
	}
	users, err := auth.ListUsersWithRole(roleName)
	if err != nil {
		return err
	}
	err = runWithPermSync(users, func() error {
		return role.AddPermissions(r.Form["permission"]...)
	})
	if err == permission.ErrInvalidPermissionName {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if perr, ok := err.(*permission.ErrPermissionNotFound); ok {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: perr.Error(),
		}
	}
	if perr, ok := err.(*permission.ErrPermissionNotAllowed); ok {
		return &errors.HTTP{
			Code:    http.StatusConflict,
			Message: perr.Error(),
		}
	}
	return err
}

func removePermissions(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleUpdate) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	permName := r.URL.Query().Get(":permission")
	role, err := permission.FindRole(roleName)
	if err != nil {
		return err
	}
	users, err := auth.ListUsersWithRole(roleName)
	if err != nil {
		return err
	}
	err = runWithPermSync(users, func() error {
		return role.RemovePermissions(permName)
	})
	return err
}

func canUseRole(t auth.Token, roleName, contextValue string) error {
	role, err := permission.FindRole(roleName)
	if err != nil {
		return err
	}
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

func assignRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleUpdateAssign) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	email := r.FormValue("email")
	contextValue := r.FormValue("context")
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return err
	}
	err = canUseRole(t, roleName, contextValue)
	if err != nil {
		return err
	}
	err = runWithPermSync([]auth.User{*user}, func() error {
		return user.AddRole(roleName, contextValue)
	})
	return err
}

func dissociateRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleUpdateDissociate) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	email := r.URL.Query().Get(":email")
	contextValue := r.URL.Query().Get("context")
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return err
	}
	err = canUseRole(t, roleName, contextValue)
	if err != nil {
		return err
	}
	err = runWithPermSync([]auth.User{*user}, func() error {
		return user.RemoveRole(roleName, contextValue)
	})
	return err
}

type permissionSchemeData struct {
	Name     string
	Contexts []string
}

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

func addDefaultRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleDefaultCreate) {
		return permission.ErrUnauthorized
	}
	err := r.ParseForm()
	if err != nil {
		return err
	}
	for evtName := range permission.RoleEventMap {
		roles := r.Form[evtName]
		for _, roleName := range roles {
			role, err := permission.FindRole(roleName)
			if err != nil {
				if err == permission.ErrRoleNotFound {
					return &errors.HTTP{
						Code:    http.StatusBadRequest,
						Message: err.Error(),
					}
				}
				return err
			}
			err = role.AddEvent(evtName)
			if err != nil {
				if _, ok := err.(permission.ErrRoleEventWrongContext); ok {
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

func removeDefaultRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleDefaultDelete) {
		return permission.ErrUnauthorized
	}
	err := r.ParseForm()
	if err != nil {
		return err
	}
	for evtName := range permission.RoleEventMap {
		roles := r.Form[evtName]
		for _, roleName := range roles {
			role, err := permission.FindRole(roleName)
			if err != nil {
				if err == permission.ErrRoleNotFound {
					return &errors.HTTP{
						Code:    http.StatusBadRequest,
						Message: err.Error(),
					}
				}
				return err
			}
			err = role.RemoveEvent(evtName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

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
