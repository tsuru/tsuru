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

func deployableApps(u *auth.User) ([]string, error) {
	perms, err := u.Permissions()
	if err != nil {
		return nil, err
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
		appNames[i] = apps[i].Name
	}
	return appNames, nil
}

func syncRepositoryApps(user *auth.User, beforeApps []string) error {
	afterApps, err := deployableApps(user)
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
	for i := range users {
		u := &users[i]
		apps, err := deployableApps(u)
		if err != nil {
			return err
		}
		usersMap[u] = apps
	}
	err := callback()
	if err != nil {
		return err
	}
	for u, apps := range usersMap {
		err = syncRepositoryApps(u, apps)
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
	users, err := auth.ListUsersWithRole(roleName)
	if err != nil {
		return err
	}
	err = runWithPermSync(users, func() error {
		return role.RemovePermissions(permName)
	})
	if err == nil {
		w.WriteHeader(http.StatusOK)
	}
	return err
}

func canUseRole(t auth.Token, roleName, contextValue string) error {
	role, err := permission.FindRole(roleName)
	if err != nil {
		return err
	}
	perms := role.PermisionsFor(contextValue)
	for _, p := range perms {
		if !permission.Check(t, p.Scheme, p.Context) {
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
	if err == nil {
		w.WriteHeader(http.StatusOK)
	}
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
