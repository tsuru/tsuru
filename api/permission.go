package api

import (
	"encoding/json"
	"github.com/tsuru/tsuru/errors"
	"net/http"

	"github.com/tsuru/tsuru/auth"
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
	return role.AddPermissions(r.Form["permission"]...)
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
	return role.RemovePermissions(permName)
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
	return user.AddRole(roleName, contextValue)
}

func dissociateRole(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	roleName := r.URL.Query().Get(":name")
	email := r.URL.Query().Get(":email")
	contextValue := r.URL.Query().Get("context")
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return err
	}
	return user.RemoveRole(roleName, contextValue)
}
