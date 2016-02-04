// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/rec"
	"github.com/tsuru/tsuru/repository"
)

const (
	nonManagedSchemeMsg = "Authentication scheme does not allow this operation."
	createDisabledMsg   = "User registration is disabled for non-admin users."
)

var createDisabledErr = &errors.HTTP{Code: http.StatusUnauthorized, Message: createDisabledMsg}

func handleAuthError(err error) error {
	if err == auth.ErrUserNotFound {
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
	var u auth.User
	err := json.NewDecoder(r.Body).Decode(&u)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	_, err = app.AuthScheme.Create(&u)
	if err != nil {
		return handleAuthError(err)
	}
	rec.Log(u.Email, "create-user")
	w.WriteHeader(http.StatusCreated)
	return nil
}

func login(w http.ResponseWriter, r *http.Request) error {
	var params map[string]string
	err := json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Invalid JSON"}
	}
	params["email"] = r.URL.Query().Get(":email")
	token, err := app.AuthScheme.Login(params)
	if err != nil {
		return handleAuthError(err)
	}
	u, err := token.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "login")
	return json.NewEncoder(w).Encode(map[string]string{"token": token.GetValue()})
}

func logout(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return app.AuthScheme.Logout(t.GetValue())
}

func changePassword(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	managed, ok := app.AuthScheme.(auth.ManagedScheme)
	if !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: nonManagedSchemeMsg}
	}
	var body map[string]string
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid JSON.",
		}
	}
	if body["old"] == "" || body["new"] == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Both the old and the new passwords are required.",
		}
	}
	err = managed.ChangePassword(t, body["old"], body["new"])
	if err != nil {
		return handleAuthError(err)
	}
	rec.Log(t.GetUserName(), "change-password")
	return nil
}

func resetPassword(w http.ResponseWriter, r *http.Request) error {
	managed, ok := app.AuthScheme.(auth.ManagedScheme)
	if !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: nonManagedSchemeMsg}
	}
	email := r.URL.Query().Get(":email")
	token := r.URL.Query().Get("token")
	u, err := auth.GetUserByEmail(email)
	if err != nil {
		if err == auth.ErrUserNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		} else if e, ok := err.(*errors.ValidationError); ok {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: e.Error()}
		}
		return err
	}
	if token == "" {
		rec.Log(email, "reset-password-gen-token")
		return managed.StartPasswordReset(u)
	}
	rec.Log(email, "reset-password")
	return managed.ResetPassword(u, token)
}

func createTeam(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var params map[string]string
	err := json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	allowed := permission.Check(t, permission.PermTeamCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	name := params["name"]
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "create-team", name)
	err = auth.CreateTeam(name, u)
	switch err {
	case auth.ErrInvalidTeamName:
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	case auth.ErrTeamAlreadyExists:
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return nil
}

func removeTeam(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.URL.Query().Get(":name")
	allowed := permission.Check(t, permission.PermTeamDelete,
		permission.Context(permission.CtxTeam, name),
	)
	if !allowed {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf(`Team "%s" not found.`, name)}
	}
	rec.Log(t.GetUserName(), "remove-team", name)
	err := auth.RemoveTeam(name)
	if err != nil {
		if _, ok := err.(*auth.ErrTeamStillUsed); ok {
			msg := fmt.Sprintf("This team cannot be removed because there are still references to it:\n%s", err)
			return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
		}
		if err == auth.ErrTeamNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf(`Team "%s" not found.`, name)}
		}
		return err
	}
	return nil
}

func teamList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	rec.Log(t.GetUserName(), "list-teams")
	permsForTeam := permission.PermissionRegistry.PermissionsWithContextType(permission.CtxTeam)
	teams, err := auth.ListTeams()
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
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	n, err := w.Write(b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write response body."}
	}
	return nil
}

type keyBody struct {
	Name  string
	Key   string
	Force bool
}

func getKeyFromBody(b io.Reader) (repository.Key, bool, error) {
	var key repository.Key
	var body keyBody
	err := json.NewDecoder(b).Decode(&body)
	if err != nil {
		return key, false, &errors.HTTP{Code: http.StatusBadRequest, Message: "Invalid JSON"}
	}
	key.Body = body.Key
	key.Name = body.Name
	return key, body.Force, nil
}

// AddKeyToUser adds a key to a user.
//
// This function is just an http wrapper around addKeyToUser. The latter function
// exists to be used in other places in the package without the http stuff (request and
// response).
func addKeyToUser(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	key, force, err := getKeyFromBody(r.Body)
	if err != nil {
		return err
	}
	if key.Body == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Missing key content"}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "add-key", key.Name, key.Body)
	err = u.AddKey(key, force)
	if err == auth.ErrKeyDisabled {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if err == repository.ErrKeyAlreadyExists {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return err
}

// RemoveKeyFromUser removes a key from a user.
//
// This function is just an http wrapper around removeKeyFromUser. The latter function
// exists to be used in other places in the package without the http stuff (request and
// response).
func removeKeyFromUser(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	key, _, err := getKeyFromBody(r.Body)
	if err != nil {
		return err
	}
	if key.Body == "" && key.Name == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Either the content or the name of the key must be provided"}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "remove-key", key.Name, key.Body)
	err = u.RemoveKey(key)
	if err == auth.ErrKeyDisabled {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if err == repository.ErrKeyNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "User does not have this key"}
	}
	return err
}

// listKeys list user's keys
func listKeys(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	keys, err := u.ListKeys()
	if err == auth.ErrKeyDisabled {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(keys)
}

// removeUser removes the user from the database and from repository server
//
// If the user is the only one in a team an error will be returned.
func removeUser(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	email := r.URL.Query().Get("user")
	if email != "" && u.Email != email {
		if !permission.Check(t, permission.PermUserDelete) {
			return permission.ErrUnauthorized
		}
		u, err = auth.GetUserByEmail(email)
		if err != nil {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
	}
	appNames, err := deployableApps(u, make(map[string]*permission.Role))
	if err != nil {
		return err
	}
	manager := repository.Manager()
	for _, name := range appNames {
		manager.RevokeAccess(name, u.Email)
	}
	rec.Log(u.Email, "remove-user")
	if err := manager.RemoveUser(u.Email); err != nil {
		log.Errorf("Failed to remove user from repository manager: %s", err)
	}
	return app.AuthScheme.Remove(u)
}

type schemeData struct {
	Name string          `json:"name"`
	Data auth.SchemeInfo `json:"data"`
}

func authScheme(w http.ResponseWriter, r *http.Request) error {
	info, err := app.AuthScheme.Info()
	if err != nil {
		return err
	}
	data := schemeData{Name: app.AuthScheme.Name(), Data: info}
	return json.NewEncoder(w).Encode(data)
}

func regenerateAPIToken(w http.ResponseWriter, r *http.Request, t auth.Token) error {
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
	apiKey, err := u.RegenerateAPIKey()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(apiKey)
}

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

func createApiUser(perms []permission.Permission, user *auth.User, roleMap map[string]*permission.Role, includeAll bool) (*apiUser, error) {
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
	if len(roleData) == 0 || (!includeAll && allGlobal) {
		return nil, nil
	}
	return &apiUser{
		Email:       user.Email,
		Roles:       roleData,
		Permissions: permData,
	}, nil
}

func listUsers(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	userEmail := r.URL.Query().Get("userEmail")
	roleName := r.URL.Query().Get("role")
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
		usrData, err := createApiUser(perms, &user, roleMap, includeAll)
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
					apiUsers = append(apiUsers, *usrData)
					break
				}
			}
		}
	}
	return json.NewEncoder(w).Encode(apiUsers)
}

func userInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	user, err := t.User()
	if err != nil {
		return err
	}
	perms, err := t.Permissions()
	if err != nil {
		return err
	}
	userData, err := createApiUser(perms, user, nil, true)
	if err != nil {
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(userData)
}
