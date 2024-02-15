// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"runtime"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/volume"
)

const (
	nonManagedSchemeMsg = "Authentication scheme does not allow this operation."
	createDisabledMsg   = "User registration is disabled for non-admin users."
)

var createDisabledErr = &errors.HTTP{Code: http.StatusUnauthorized, Message: createDisabledMsg}

func handleAuthError(err error) error {
	if err == authTypes.ErrUserNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	switch err.(type) {
	case *errors.ValidationError:
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	case *errors.ConflictError:
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	case *errors.NotAuthorizedError:
		return &errors.HTTP{Code: http.StatusForbidden, Message: err.Error()}
	case auth.AuthenticationFailure:
		return &errors.HTTP{Code: http.StatusUnauthorized, Message: err.Error()}
	default:
		return err
	}
}

func userTarget(u string) event.Target {
	return event.Target{Type: event.TargetTypeUser, Value: u}
}

func teamTarget(t string) event.Target {
	return event.Target{Type: event.TargetTypeTeam, Value: t}
}

// title: user create
// path: /users
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	201: User created
//	400: Invalid data
//	401: Unauthorized
//	403: Forbidden
//	409: User already exists
func createUser(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	registrationEnabled, _ := config.GetBool("auth:user-registration")
	if !registrationEnabled {
		token := r.Header.Get("Authorization")
		t, err := app.AuthScheme.Auth(ctx, token)
		if err != nil {
			return createDisabledErr
		}
		if !permission.Check(t, permission.PermUserCreate) {
			return createDisabledErr
		}
	}
	email := InputValue(r, "email")
	password := InputValue(r, "password")
	evt, err := event.New(&event.Opts{
		Target:     userTarget(email),
		Kind:       permission.PermUserCreate,
		RawOwner:   event.Owner{Type: event.OwnerTypeUser, Name: email},
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r, "password")),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permTypes.CtxUser, email)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u := auth.User{
		Email:    email,
		Password: password,
	}
	_, err = app.AuthScheme.Create(ctx, &u)
	if err != nil {
		return handleAuthError(err)
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

// title: login
// path: /auth/login
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	403: Forbidden
//	404: Not found
func login(w http.ResponseWriter, r *http.Request) (err error) {
	ctx := r.Context()
	params := map[string]string{
		"email": r.URL.Query().Get(":email"),
	}
	fields := InputFields(r)
	for key, values := range fields {
		params[key] = values[0]
	}
	token, err := app.AuthScheme.Login(ctx, params)
	if err != nil {
		return handleAuthError(err)
	}
	return json.NewEncoder(w).Encode(map[string]string{"token": token.GetValue()})
}

// title: webLogin
// path: /auth/webLogin
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	403: Forbidden
//	404: Not found
func webLogin(w http.ResponseWriter, r *http.Request) (err error) {
	ctx := r.Context()
	body, err := r.GetBody()
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(body)
	type webResponseBody struct {
		email string
		token string
	}
	var responseBody webResponseBody
	err = decoder.Decode(&responseBody)
	if err != nil {
		return err
	}
	err = app.AuthScheme.WebLogin(ctx, responseBody.email, responseBody.token)
	if err != nil {
		return handleAuthError(err)
	}
	return json.NewEncoder(w).Encode(map[string]string{"token": responseBody.token})
}

// title: logout
// path: /users/tokens
// method: DELETE
// responses:
//
//	200: Ok
func logout(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	return app.AuthScheme.Logout(r.Context(), t.GetValue())
}

// title: change password
// path: /users/password
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	403: Forbidden
//	404: Not found
func changePassword(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	managed, ok := app.AuthScheme.(auth.ManagedScheme)
	if !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: nonManagedSchemeMsg}
	}
	evt, err := event.New(&event.Opts{
		Target:     userTarget(t.GetUserName()),
		Kind:       permission.PermUserUpdatePassword,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permTypes.CtxUser, t.GetUserName())),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	oldPassword := InputValue(r, "old")
	newPassword := InputValue(r, "new")
	confirmPassword := InputValue(r, "confirm")
	if oldPassword == "" || newPassword == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Both the old and the new passwords are required.",
		}
	}
	if newPassword != confirmPassword {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "New password and password confirmation didn't match.",
		}
	}
	err = managed.ChangePassword(ctx, t, oldPassword, newPassword)
	if err != nil {
		return handleAuthError(err)
	}
	return nil
}

// title: reset password
// path: /users/{email}/password
// method: POST
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	403: Forbidden
//	404: Not found
func resetPassword(w http.ResponseWriter, r *http.Request) (err error) {
	ctx := r.Context()
	managed, ok := app.AuthScheme.(auth.ManagedScheme)
	if !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: nonManagedSchemeMsg}
	}
	email := r.URL.Query().Get(":email")
	token := InputValue(r, "token")
	evt, err := event.New(&event.Opts{
		Target:     userTarget(email),
		Kind:       permission.PermUserUpdateReset,
		RawOwner:   event.Owner{Type: event.OwnerTypeUser, Name: email},
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permTypes.CtxUser, email)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u, err := auth.GetUserByEmail(email)
	if err != nil {
		if err == authTypes.ErrUserNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	if token == "" {
		return managed.StartPasswordReset(ctx, u)
	}
	return managed.ResetPassword(ctx, u, token)
}

var teamRenameFns = []func(ctx context.Context, oldName, newName string) error{
	app.RenameTeam,
	service.RenameServiceTeam,
	service.RenameServiceInstanceTeam,
	volume.RenameTeam,
	pool.RenamePoolTeam,
}

// title: team update
// path: /teams/{name}
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//
//	200: Team updated
//	400: Invalid data
//	401: Unauthorized
//	404: Team not found
func updateTeam(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	name := r.URL.Query().Get(":name")
	type teamChange struct {
		NewName string
		Tags    []string
	}
	changeRequest := teamChange{}
	if err := ParseInput(r, &changeRequest); err != nil {
		return err
	}
	tags, _ := InputValues(r, "tag")
	changeRequest.Tags = append(changeRequest.Tags, tags...) // for compatibility
	allowed := permission.Check(t, permission.PermTeamUpdate,
		permission.Context(permTypes.CtxTeam, name),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	_, err := servicemanager.Team.FindByName(ctx, name)
	if err != nil {
		if err == authTypes.ErrTeamNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	evt, err := event.New(&event.Opts{
		Target:     teamTarget(name),
		Kind:       permission.PermTeamUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permTypes.CtxTeam, name)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if changeRequest.NewName == "" {
		return servicemanager.Team.Update(ctx, name, changeRequest.Tags)
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	err = servicemanager.Team.Create(ctx, changeRequest.NewName, changeRequest.Tags, u)
	if err != nil {
		return err
	}
	var toRollback []func(ctx context.Context, oldName, newName string) error
	defer func() {
		if err == nil {
			return
		}
		rollbackErr := servicemanager.Team.Remove(ctx, changeRequest.NewName)
		if rollbackErr != nil {
			log.Errorf("error rolling back team creation from %v to %v", name, changeRequest.NewName)
		}
		for _, rollbackFn := range toRollback {
			rollbackErr := rollbackFn(ctx, changeRequest.NewName, name)
			if rollbackErr != nil {
				fnName := runtime.FuncForPC(reflect.ValueOf(rollbackFn).Pointer()).Name()
				log.Errorf("error rolling back team name change in %v from %q to %q", fnName, name, changeRequest.NewName)
			}
		}
	}()
	for _, fn := range teamRenameFns {
		err = fn(ctx, name, changeRequest.NewName)
		if err != nil {
			return err
		}
		toRollback = append(toRollback, fn)
	}
	return servicemanager.Team.Remove(ctx, name)
}

// title: team create
// path: /teams
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	201: Team created
//	400: Invalid data
//	401: Unauthorized
//	409: Team already exists
func createTeam(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	allowed := permission.Check(t, permission.PermTeamCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	var team authTypes.Team
	if err := ParseInput(r, &team); err != nil {
		return err
	}
	tags, _ := InputValues(r, "tag")
	team.Tags = append(team.Tags, tags...) // for compatibility
	evt, err := event.New(&event.Opts{
		Target:     teamTarget(team.Name),
		Kind:       permission.PermTeamCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permTypes.CtxTeam, team.Name)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u, err := t.User()
	if err != nil {
		return err
	}
	err = servicemanager.Team.Create(ctx, team.Name, team.Tags, u)
	switch err {
	case authTypes.ErrInvalidTeamName:
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	case authTypes.ErrTeamAlreadyExists:
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

// title: remove team
// path: /teams/{name}
// method: DELETE
// responses:
//
//	200: Team removed
//	401: Unauthorized
//	403: Forbidden
//	404: Not found
func removeTeam(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	name := r.URL.Query().Get(":name")
	allowed := permission.Check(t, permission.PermTeamDelete,
		permission.Context(permTypes.CtxTeam, name),
	)
	if !allowed {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf(`Team "%s" not found.`, name)}
	}
	evt, err := event.New(&event.Opts{
		Target:     teamTarget(name),
		Kind:       permission.PermTeamDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permTypes.CtxTeam, name)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = servicemanager.Team.Remove(ctx, name)
	if err != nil {
		if _, ok := err.(*authTypes.ErrTeamStillUsed); ok {
			msg := fmt.Sprintf("This team cannot be removed because there are still references to it:\n%s", err)
			return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
		}
		if err == authTypes.ErrTeamNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf(`Team "%s" not found.`, name)}
		}
		return err
	}
	return nil
}

// title: team list
// path: /teams
// method: GET
// produce: application/json
// responses:
//
//	200: List teams
//	204: No content
//	401: Unauthorized
func teamList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	permsForTeam := permission.PermissionRegistry.PermissionsWithContextType(permTypes.CtxTeam)
	teams, err := servicemanager.Team.List(ctx)
	if err != nil {
		return err
	}
	teamsMap := map[string]authTypes.Team{}
	permsMap := map[string][]string{}
	perms, err := t.Permissions()
	if err != nil {
		return err
	}
	for _, team := range teams {
		teamsMap[team.Name] = team
		teamCtx := permission.Context(permTypes.CtxTeam, team.Name)
		var parent *permission.PermissionScheme
		for _, p := range permsForTeam {
			if parent != nil && parent.IsParent(p) {
				continue
			}
			if permission.CheckFromPermList(perms, p, teamCtx) {
				parent = p
				permsMap[team.Name] = append(permsMap[team.Name], p.FullName())
			}
		}
	}
	if len(permsMap) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	var result []map[string]interface{}
	for name, permissions := range permsMap {
		result = append(result, map[string]interface{}{
			"name":        name,
			"tags":        teamsMap[name].Tags,
			"permissions": permissions,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(result)
}

// title: team info
// path: /teams/{name}
// method: GET
// produce: application/json
// responses:
//
//	200: Info team
//	404: Not found
//	401: Unauthorized
func teamInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	teamName := r.URL.Query().Get(":name")
	team, err := servicemanager.Team.FindByName(ctx, teamName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	canRead := permission.Check(t, permission.PermTeamRead)
	if !canRead {
		return permission.ErrUnauthorized
	}
	apps, err := app.List(ctx, &app.Filter{
		Extra:     map[string][]string{"teams": {team.Name}},
		TeamOwner: team.Name})
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	pools, err := pool.ListPoolsForTeam(ctx, team.Name)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	users, err := auth.ListUsers()
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	includedUsers := make([]*apiUser, 0)

	filteredRolesMap, err := permission.ListRolesWithPermissionWithContextMap(permTypes.CtxTeam)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}

	canInclude := permission.Check(t, permission.PermTeam)
	if canInclude {
		for _, user := range users {
			for _, roleInstance := range user.Roles {
				_, found := filteredRolesMap[roleInstance.Name]
				if found && roleInstance.ContextValue == teamName {
					roleMap := make(map[string]*permission.Role)
					perms, err := t.Permissions()
					if err != nil {
						return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
					}
					userData, err := createAPIUser(perms, &user, roleMap, canInclude)
					if err != nil {
						return &errors.HTTP{Code: http.StatusInternalServerError, Message: err.Error()}
					}
					includedUsers = append(includedUsers, userData)
				}
			}
		}
	}
	result := map[string]interface{}{
		"name":  team.Name,
		"tags":  team.Tags,
		"users": includedUsers,
		"pools": pools,
		"apps":  apps,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(result)
}

// title: remove user
// path: /users
// method: DELETE
// responses:
//
//	200: User removed
//	401: Unauthorized
//	404: Not found
func removeUser(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	email := r.URL.Query().Get("user")
	if email == "" {
		email = t.GetUserName()
	}
	allowed := permission.Check(t, permission.PermUserDelete,
		permission.Context(permTypes.CtxUser, email),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     userTarget(email),
		Kind:       permission.PermUserDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permTypes.CtxUser, email)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u, err := auth.GetUserByEmail(email)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return app.AuthScheme.Remove(ctx, u)
}

// title: get auth scheme
// path: /auth/scheme
// method: GET
// produce: application/json
// responses:
//
//	200: OK
func authScheme(w http.ResponseWriter, r *http.Request) error {
	info, err := app.AuthScheme.Info(r.Context())
	if err != nil {
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(info)
}

// title: regenerate token
// path: /users/api-key
// method: POST
// produce: application/json
// responses:
//
//	200: OK
//	401: Unauthorized
//	404: User not found
func regenerateAPIToken(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	email := r.URL.Query().Get("user")
	if email == "" {
		email = t.GetUserName()
	}
	allowed := permission.Check(t, permission.PermApikeyUpdate,
		permission.Context(permTypes.CtxUser, email),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     userTarget(email),
		Kind:       permission.PermApikeyUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permTypes.CtxUser, email)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u, err := auth.GetUserByEmail(email)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	apiKey, err := u.RegenerateAPIKey()
	if err != nil {
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(apiKey)
}

// title: show token
// path: /users/api-key
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	401: Unauthorized
//	404: User not found
func showAPIToken(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := auth.ConvertNewUser(t.User())
	if err != nil {
		return err
	}

	email := r.URL.Query().Get("user")

	emailToCheckPerm := email
	if emailToCheckPerm == "" {
		emailToCheckPerm = u.Email
	}

	if !permission.Check(t, permission.PermApikeyRead, permission.Context(permTypes.CtxUser, emailToCheckPerm)) {
		return permission.ErrUnauthorized
	}

	if email != "" {
		u, err = auth.GetUserByEmail(email)
		if err != nil {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
	}
	apiKey, err := u.ShowAPIKey()
	if err != nil {
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(apiKey)
}

type rolePermissionData struct {
	Name         string
	ContextType  string
	ContextValue string
	Group        string `json:",omitempty"`
}

type apiUser struct {
	Email       string
	Roles       []rolePermissionData
	Permissions []rolePermissionData
	Groups      []string
}

func createAPIUser(perms []permission.Permission, user *auth.User, roleMap map[string]*permission.Role, includeAll bool) (*apiUser, error) {
	if roleMap == nil {
		roleMap = make(map[string]*permission.Role)
	}
	allGlobal := true

	apiUsr := &apiUser{
		Email:  user.Email,
		Groups: user.Groups,
		Roles:  make([]rolePermissionData, 0, len(user.Roles)),
	}

	for _, userRole := range user.Roles {
		isGlobal, err := expandRoleData(perms, userRole, apiUsr, roleMap, includeAll, "")
		if err != nil {
			return nil, err
		}
		if !isGlobal {
			allGlobal = false
		}
	}

	groups, err := user.UserGroups()
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		for _, groupRole := range group.Roles {
			isGlobal, err := expandRoleData(perms, groupRole, apiUsr, roleMap, includeAll, group.Name)
			if err != nil {
				return nil, err
			}
			if !isGlobal {
				allGlobal = false
			}
		}
	}

	if !includeAll && allGlobal {
		return nil, nil
	}
	return apiUsr, nil
}

func expandRoleData(perms []permission.Permission, userRole authTypes.RoleInstance, user *apiUser, roleMap map[string]*permission.Role, includeAll bool, group string) (bool, error) {
	role := roleMap[userRole.Name]
	if role == nil {
		r, err := permission.FindRole(userRole.Name)
		if err != nil {
			return true, err
		}
		role = &r
		roleMap[userRole.Name] = role
	}
	allPermsMatch := true
	permissions := role.PermissionsFor(userRole.ContextValue)
	if len(permissions) == 0 && !includeAll {
		return true, nil
	}
	rolePerms := make([]rolePermissionData, len(permissions))
	for i, p := range permissions {
		if perms != nil && allPermsMatch && !permission.CheckFromPermList(perms, p.Scheme, p.Context) {
			allPermsMatch = false
			break
		}
		rolePerms[i] = rolePermissionData{
			Name:         p.Scheme.FullName(),
			ContextType:  string(p.Context.CtxType),
			ContextValue: p.Context.Value,
			Group:        group,
		}
	}
	if !allPermsMatch {
		return true, nil
	}
	user.Roles = append(user.Roles, rolePermissionData{
		Name:         userRole.Name,
		ContextType:  string(role.ContextType),
		ContextValue: userRole.ContextValue,
		Group:        group,
	})
	user.Permissions = append(user.Permissions, rolePerms...)
	return role.ContextType == permTypes.CtxGlobal, nil
}

// title: user list
// path: /users
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	401: Unauthorized
func listUsers(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	userEmail := r.URL.Query().Get("userEmail")
	roleName := r.URL.Query().Get("role")
	contextValue := r.URL.Query().Get("context")
	users, err := auth.ListUsers()
	if err != nil {
		return err
	}
	apiUsers := make([]apiUser, 0, len(users))
	roleMap := make(map[string]*permission.Role)
	includeAll := permission.Check(t, permission.PermUserUpdate)
	perms, err := t.Permissions()
	if err != nil {
		return err
	}
	for _, user := range users {
		usrData, err := createAPIUser(perms, &user, roleMap, includeAll)
		if err != nil {
			return err
		}
		if usrData == nil {
			continue
		}
		if userEmail == "" && roleName == "" {
			apiUsers = append(apiUsers, *usrData)
		}
		if userEmail != "" && usrData.Email == userEmail {
			apiUsers = append(apiUsers, *usrData)
		}
		if roleName != "" {
			for _, role := range usrData.Roles {
				if role.Name == roleName {
					if contextValue != "" && role.ContextValue == contextValue {
						apiUsers = append(apiUsers, *usrData)
						break
					}
					if contextValue == "" {
						apiUsers = append(apiUsers, *usrData)
						break
					}
				}
			}
		}
	}
	if len(apiUsers) == 0 {
		if contextValue != "" {
			return &errors.HTTP{Code: http.StatusNotFound, Message: "Wrong context being passed."}
		}
		user, err := auth.ConvertNewUser(t.User())
		if err != nil {
			return err
		}
		perm, err := user.Permissions()
		if err != nil {
			return err
		}
		userData, err := createAPIUser(perm, user, nil, true)
		if err != nil {
			return err
		}
		apiUsers = append(apiUsers, *userData)
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(apiUsers)
}

// title: user info
// path: /users/info
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	401: Unauthorized
func userInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	user, err := auth.ConvertNewUser(t.User())
	if err != nil {
		return err
	}
	perms, err := t.Permissions()
	if err != nil {
		return err
	}
	userData, err := createAPIUser(perms, user, nil, true)
	if err != nil {
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(userData)
}

// title: team users
// path: /teams/{name}/users
// method: GET
// produce: application/json
// responses:
//
//	200: team users
//	404: Not found
//	401: Unauthorized
func teamUserList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	teamName := r.URL.Query().Get(":name")
	_, err := servicemanager.Team.FindByName(ctx, teamName)
	if err == authTypes.ErrTeamNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if err != nil {
		return err
	}

	allowed := permission.Check(t, permission.PermTeamRead,
		permission.Context(permTypes.CtxTeam, teamName),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	allRoles, err := permission.ListRoles()
	if err != nil {
		return err
	}

	teamRoles := []string{}
	roleMap := map[string]bool{}
	for _, role := range allRoles {
		if role.ContextType == permTypes.CtxTeam {
			teamRoles = append(teamRoles, role.Name)
			roleMap[role.Name] = true
		}
	}

	users, err := auth.ListUsersWithRolesAndContext(teamRoles, teamName)
	if err != nil {
		return err
	}

	result := []teamUserItem{}
	for _, user := range users {
		userContextRoles := []string{}
		alreadyPushedRoles := map[string]bool{}
		for _, role := range user.Roles {
			if roleMap[role.Name] && role.ContextValue == teamName && !alreadyPushedRoles[role.Name] {
				userContextRoles = append(userContextRoles, role.Name)
				alreadyPushedRoles[role.Name] = true
			}
		}

		result = append(result, teamUserItem{Email: user.Email, Roles: userContextRoles})
	}

	return json.NewEncoder(w).Encode(result)
}

// title: team groups
// path: /teams/{name}/groups
// method: GET
// produce: application/json
// responses:
//
//	200: team groups
//	404: Not found
//	401: Unauthorized
func teamGroupList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	teamName := r.URL.Query().Get(":name")
	_, err := servicemanager.Team.FindByName(ctx, teamName)
	if err == authTypes.ErrTeamNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if err != nil {
		return err
	}

	allowed := permission.Check(t, permission.PermTeamRead,
		permission.Context(permTypes.CtxTeam, teamName),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	allRoles, err := permission.ListRoles()
	if err != nil {
		return err
	}

	roleMap := map[string]bool{}
	for _, role := range allRoles {
		if role.ContextType == permTypes.CtxTeam {
			roleMap[role.Name] = true
		}
	}

	allGroups, err := servicemanager.AuthGroup.List(nil)
	if err != nil {
		return err
	}

	result := []teamGroupItem{}
	for _, group := range allGroups {
		groupContextRoles := []string{}
		alreadyPushedRoles := map[string]bool{}

		for _, role := range group.Roles {
			if roleMap[role.Name] && role.ContextValue == teamName && !alreadyPushedRoles[role.Name] {
				groupContextRoles = append(groupContextRoles, role.Name)
				alreadyPushedRoles[role.Name] = true
			}
		}

		if len(groupContextRoles) > 0 {
			result = append(result, teamGroupItem{Group: group.Name, Roles: groupContextRoles})
		}
	}

	return json.NewEncoder(w).Encode(result)
}

type teamUserItem struct {
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

type teamGroupItem struct {
	Group string   `json:"group"`
	Roles []string `json:"roles"`
}
