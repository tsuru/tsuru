package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
)

func addRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleCreate) {
		return permission.ErrUnauthorized
	}
	_, err := permission.NewRole(r.FormValue("name"), r.FormValue("context"))
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

func removeRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleDelete) {
		return permission.ErrUnauthorized
	}
	err := permission.DestroyRole(r.URL.Query().Get(":name"))
	if err == permission.ErrRoleNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if err == nil {
		w.WriteHeader(http.StatusOK)
	}
	return err
}

func listRoles(w http.ResponseWriter, r *http.Request, t auth.Token) error {
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
	err = role.AddPermissions(r.Form["permission"]...)
	if err == nil {
		w.WriteHeader(http.StatusOK)
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
	err = role.RemovePermissions(permName)
	if err == nil {
		w.WriteHeader(http.StatusOK)
	}
	return err
}

func assignRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermRoleAssign) {
		return permission.ErrUnauthorized
	}
	roleName := r.URL.Query().Get(":name")
	email := r.FormValue("email")
	contextValue := r.FormValue("context")
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return err
	}
	err = user.AddRole(roleName, contextValue)
	if err == nil {
		w.WriteHeader(http.StatusOK)
	}
	return err
}

func dissociateRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	roleName := r.URL.Query().Get(":name")
	email := r.URL.Query().Get(":email")
	contextValue := r.URL.Query().Get("context")
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return err
	}
	err = user.RemoveRole(roleName, contextValue)
	if err == nil {
		w.WriteHeader(http.StatusOK)
	}
	return err
}

type permissionSchemeData struct {
	Name     string
	Contexts []string
}

func listPermissions(w http.ResponseWriter, r *http.Request, t auth.Token) error {
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
