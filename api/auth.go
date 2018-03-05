// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"runtime"

	"github.com/ajg/form"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
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
//   201: User created
//   400: Invalid data
//   401: Unauthorized
//   403: Forbidden
//   409: User already exists
func createUser(w http.ResponseWriter, r *http.Request) error {
	registrationEnabled, _ := config.GetBool("auth:user-registration")
	if !registrationEnabled {
		token := r.Header.Get("Authorization")
		t, err := app.AuthScheme.Auth(token)
		if err != nil {
			return createDisabledErr
		}
		if !permission.Check(t, permission.PermUserCreate) {
			return createDisabledErr
		}
	}
	email := r.FormValue("email")
	password := r.FormValue("password")
	delete(r.Form, "password")
	evt, err := event.New(&event.Opts{
		Target:     userTarget(email),
		Kind:       permission.PermUserCreate,
		RawOwner:   event.Owner{Type: event.OwnerTypeUser, Name: email},
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permission.CtxUser, email)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u := auth.User{
		Email:    email,
		Password: password,
	}
	_, err = app.AuthScheme.Create(&u)
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
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   403: Forbidden
//   404: Not found
func login(w http.ResponseWriter, r *http.Request) (err error) {
	params := map[string]string{
		"email": r.URL.Query().Get(":email"),
	}
	err = r.ParseForm()
	if err != nil {
		return err
	}
	for key := range r.Form {
		params[key] = r.FormValue(key)
	}
	token, err := app.AuthScheme.Login(params)
	if err != nil {
		return handleAuthError(err)
	}
	return json.NewEncoder(w).Encode(map[string]string{"token": token.GetValue()})
}

// title: logout
// path: /users/tokens
// method: DELETE
// responses:
//   200: Ok
func logout(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	return app.AuthScheme.Logout(t.GetValue())
}

// title: change password
// path: /users/password
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   403: Forbidden
//   404: Not found
func changePassword(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	managed, ok := app.AuthScheme.(auth.ManagedScheme)
	if !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: nonManagedSchemeMsg}
	}
	evt, err := event.New(&event.Opts{
		Target:  userTarget(t.GetUserName()),
		Kind:    permission.PermUserUpdatePassword,
		Owner:   t,
		Allowed: event.Allowed(permission.PermUserReadEvents, permission.Context(permission.CtxUser, t.GetUserName())),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	oldPassword := r.FormValue("old")
	newPassword := r.FormValue("new")
	confirmPassword := r.FormValue("confirm")
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
	err = managed.ChangePassword(t, oldPassword, newPassword)
	if err != nil {
		return handleAuthError(err)
	}
	return nil
}

// title: reset password
// path: /users/{email}/password
// method: POST
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   403: Forbidden
//   404: Not found
func resetPassword(w http.ResponseWriter, r *http.Request) (err error) {
	managed, ok := app.AuthScheme.(auth.ManagedScheme)
	if !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: nonManagedSchemeMsg}
	}
	r.ParseForm()
	email := r.URL.Query().Get(":email")
	token := r.FormValue("token")
	evt, err := event.New(&event.Opts{
		Target:     userTarget(email),
		Kind:       permission.PermUserUpdateReset,
		RawOwner:   event.Owner{Type: event.OwnerTypeUser, Name: email},
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permission.CtxUser, email)),
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
		return managed.StartPasswordReset(u)
	}
	return managed.ResetPassword(u, token)
}

var teamRenameFns = []func(oldName, newName string) error{
	app.RenameTeam,
	service.RenameServiceTeam,
	service.RenameServiceInstanceTeam,
	volume.RenameTeam,
	pool.RenamePoolTeam,
}

// title: team update
// path: /teams/{name}
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Team updated
//   400: Invalid data
//   401: Unauthorized
//   404: Team not found
func updateTeam(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	name := r.URL.Query().Get(":name")
	type teamChange struct {
		NewName string
	}
	changeRequest := teamChange{}
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	err = dec.DecodeValues(&changeRequest, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if changeRequest.NewName == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "new team name cannot be empty"}
	}
	allowed := permission.Check(t, permission.PermTeamUpdate,
		permission.Context(permission.CtxTeam, name),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	_, err = servicemanager.Team.FindByName(name)
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
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permission.CtxTeam, name)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u, err := t.User()
	if err != nil {
		return err
	}
	user := authTypes.User(*u)
	err = servicemanager.Team.Create(changeRequest.NewName, &user)
	if err != nil {
		return err
	}
	var toRollback []func(oldName, newName string) error
	defer func() {
		if err == nil {
			return
		}
		rollbackErr := servicemanager.Team.Remove(changeRequest.NewName)
		if rollbackErr != nil {
			log.Errorf("error rolling back team creation from %v to %v", name, changeRequest.NewName)
		}
		for _, rollbackFn := range toRollback {
			rollbackErr := rollbackFn(changeRequest.NewName, name)
			if rollbackErr != nil {
				fnName := runtime.FuncForPC(reflect.ValueOf(rollbackFn).Pointer()).Name()
				log.Errorf("error rolling back team name change in %v from %q to %q", fnName, name, changeRequest.NewName)
			}
		}
	}()
	for _, fn := range teamRenameFns {
		err = fn(name, changeRequest.NewName)
		if err != nil {
			return err
		}
		toRollback = append(toRollback, fn)
	}
	return servicemanager.Team.Remove(name)
}

// title: team create
// path: /teams
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   201: Team created
//   400: Invalid data
//   401: Unauthorized
//   409: Team already exists
func createTeam(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermTeamCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	name := r.FormValue("name")
	if name == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: authTypes.ErrInvalidTeamName.Error()}
	}
	evt, err := event.New(&event.Opts{
		Target:     teamTarget(name),
		Kind:       permission.PermTeamCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permission.CtxTeam, name)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u, err := t.User()
	if err != nil {
		return err
	}
	user := authTypes.User(*u)
	err = servicemanager.Team.Create(name, &user)
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
//   200: Team removed
//   401: Unauthorized
//   403: Forbidden
//   404: Not found
func removeTeam(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	name := r.URL.Query().Get(":name")
	allowed := permission.Check(t, permission.PermTeamDelete,
		permission.Context(permission.CtxTeam, name),
	)
	if !allowed {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf(`Team "%s" not found.`, name)}
	}
	evt, err := event.New(&event.Opts{
		Target:     teamTarget(name),
		Kind:       permission.PermTeamDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permission.CtxTeam, name)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = servicemanager.Team.Remove(name)
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
//   200: List teams
//   204: No content
//   401: Unauthorized
func teamList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	permsForTeam := permission.PermissionRegistry.PermissionsWithContextType(permission.CtxTeam)
	teams, err := servicemanager.Team.List()
	if err != nil {
		return err
	}
	teamsMap := map[string][]string{}
	perms, err := t.Permissions()
	if err != nil {
		return err
	}
	for _, team := range teams {
		teamCtx := permission.Context(permission.CtxTeam, team.Name)
		var parent *permission.PermissionScheme
		for _, p := range permsForTeam {
			if parent != nil && parent.IsParent(p) {
				continue
			}
			if permission.CheckFromPermList(perms, p, teamCtx) {
				parent = p
				teamsMap[team.Name] = append(teamsMap[team.Name], p.FullName())
			}
		}
	}
	if len(teamsMap) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	var result []map[string]interface{}
	for name, permissions := range teamsMap {
		result = append(result, map[string]interface{}{
			"name":        name,
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
//   200: Info team
//   404: Not found
//   401: Unauthorized
func teamInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	teamName := r.URL.Query().Get(":name")
	team, err := servicemanager.Team.FindByName(teamName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	canRead := permission.Check(t, permission.PermTeamRead)
	if !canRead {
		return permission.ErrUnauthorized
	}
	apps, err := app.List(&app.Filter{
		Extra:     map[string][]string{"teams": {team.Name}},
		TeamOwner: team.Name})
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	pools, err := pool.ListPoolsForTeam(team.Name)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	users, err := auth.ListUsers()
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	cachedRoles := make(map[string]permission.Role)
	includedUsers := make([]*apiUser, 0)
	for _, user := range users {
		for _, roleInstance := range user.Roles {
			role, ok := cachedRoles[roleInstance.Name]
			if !ok {
				roleFound, err := permission.FindRole(roleInstance.Name)
				if err != nil {
					return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
				}
				cachedRoles[roleInstance.Name] = roleFound
				role = cachedRoles[roleInstance.Name]
			}
			if role.ContextType == permission.CtxGlobal || (role.ContextType == permission.CtxTeam && roleInstance.ContextValue == team.Name) {
				canInclude := permission.Check(t, permission.PermTeam)
				if canInclude {
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
					break
				}
			}
		}
	}
	result := map[string]interface{}{
		"name":  team.Name,
		"users": includedUsers,
		"pools": pools,
		"apps":  apps,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(result)
}

// title: add key
// path: /users/keys
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   409: Key already exists
func addKeyToUser(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	key := repository.Key{
		Body: r.FormValue("key"),
		Name: r.FormValue("name"),
	}
	var force bool
	if r.FormValue("force") == "true" {
		force = true
	}
	allowed := permission.Check(t, permission.PermUserUpdateKeyAdd,
		permission.Context(permission.CtxUser, t.GetUserName()),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     userTarget(t.GetUserName()),
		Kind:       permission.PermUserUpdateKeyAdd,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permission.CtxUser, t.GetUserName())),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if key.Body == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Missing key content"}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	err = u.AddKey(key, force)
	if err == authTypes.ErrKeyDisabled || err == repository.ErrUserNotFound {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if err == repository.ErrKeyAlreadyExists {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return err
}

// title: remove key
// path: /users/keys/{key}
// method: DELETE
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Not found
func removeKeyFromUser(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	key := repository.Key{
		Name: r.URL.Query().Get(":key"),
	}
	if key.Name == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Either the content or the name of the key must be provided"}
	}
	allowed := permission.Check(t, permission.PermUserUpdateKeyRemove,
		permission.Context(permission.CtxUser, t.GetUserName()),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     userTarget(t.GetUserName()),
		Kind:       permission.PermUserUpdateKeyRemove,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permission.CtxUser, t.GetUserName())),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u, err := t.User()
	if err != nil {
		return err
	}
	err = u.RemoveKey(key)
	if err == authTypes.ErrKeyDisabled {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if err == repository.ErrKeyNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "User does not have this key"}
	}
	return err
}

// title: list keys
// path: /users/keys
// method: GET
// produce: application/json
// responses:
//   200: OK
//   400: Invalid data
//   401: Unauthorized
func listKeys(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	keys, err := u.ListKeys()
	if err == authTypes.ErrKeyDisabled {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if err != nil {
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(keys)
}

// title: remove user
// path: /users
// method: DELETE
// responses:
//   200: User removed
//   401: Unauthorized
//   404: Not found
func removeUser(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	email := r.URL.Query().Get("user")
	if email == "" {
		email = t.GetUserName()
	}
	allowed := permission.Check(t, permission.PermUserDelete,
		permission.Context(permission.CtxUser, email),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     userTarget(email),
		Kind:       permission.PermUserDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permission.CtxUser, email)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	u, err := auth.GetUserByEmail(email)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	appNames, err := deployableApps(u, make(map[string]*permission.Role))
	if err != nil {
		return err
	}
	manager := repository.Manager()
	for _, name := range appNames {
		manager.RevokeAccess(name, u.Email)
	}
	if err := manager.RemoveUser(u.Email); err != nil {
		log.Errorf("Failed to remove user from repository manager: %s", err)
	}
	return app.AuthScheme.Remove(u)
}

type schemeData struct {
	Name string          `json:"name"`
	Data auth.SchemeInfo `json:"data"`
}

// title: get auth scheme
// path: /auth/scheme
// method: GET
// produce: application/json
// responses:
//   200: OK
func authScheme(w http.ResponseWriter, r *http.Request) error {
	info, err := app.AuthScheme.Info()
	if err != nil {
		return err
	}
	data := schemeData{Name: app.AuthScheme.Name(), Data: info}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
}

// title: regenerate token
// path: /users/api-key
// method: POST
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: User not found
func regenerateAPIToken(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	email := r.URL.Query().Get("user")
	if email == "" {
		email = t.GetUserName()
	}
	allowed := permission.Check(t, permission.PermUserUpdateToken,
		permission.Context(permission.CtxUser, email),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     userTarget(email),
		Kind:       permission.PermUserUpdateToken,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permission.CtxUser, email)),
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
//   200: OK
//   401: Unauthorized
//   404: User not found
func showAPIToken(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	email := r.URL.Query().Get("user")
	if email != "" {
		if !permission.Check(t, permission.PermUserUpdateToken) {
			return permission.ErrUnauthorized
		}
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
}

type apiUser struct {
	Email       string
	Roles       []rolePermissionData
	Permissions []rolePermissionData
}

func createAPIUser(perms []permission.Permission, user *auth.User, roleMap map[string]*permission.Role, includeAll bool) (*apiUser, error) {
	var permData []rolePermissionData
	roleData := make([]rolePermissionData, 0, len(user.Roles))
	if roleMap == nil {
		roleMap = make(map[string]*permission.Role)
	}
	allGlobal := true
	for _, userRole := range user.Roles {
		role := roleMap[userRole.Name]
		if role == nil {
			r, err := permission.FindRole(userRole.Name)
			if err != nil {
				return nil, err
			}
			role = &r
			roleMap[userRole.Name] = role
		}
		allPermsMatch := true
		permissions := role.PermissionsFor(userRole.ContextValue)
		if len(permissions) == 0 && !includeAll {
			continue
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
			}
		}
		if !allPermsMatch {
			continue
		}
		roleData = append(roleData, rolePermissionData{
			Name:         userRole.Name,
			ContextType:  string(role.ContextType),
			ContextValue: userRole.ContextValue,
		})
		permData = append(permData, rolePerms...)
		if role.ContextType != permission.CtxGlobal {
			allGlobal = false
		}
	}
	if !includeAll && allGlobal {
		return nil, nil
	}
	return &apiUser{
		Email:       user.Email,
		Roles:       roleData,
		Permissions: permData,
	}, nil
}

// title: user list
// path: /users
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
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
		user, err := t.User()
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
//   200: OK
//   401: Unauthorized
func userInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	user, err := t.User()
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
