// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	_ "crypto/sha256"
	"fmt"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/validation"
)

type User struct {
	Quota     quota.Quota
	Email     string
	Password  string
	APIKey    string
	Roles     []authTypes.RoleInstance `bson:",omitempty"`
	Groups    []string                 `bson:",omitempty"`
	FromToken bool                     `bson:",omitempty"`
	Disabled  bool                     `bson:",omitempty"`

	APIKeyLastAccess   time.Time `bson:"apikey_last_access"`
	APIKeyUsageCounter int64     `bson:"apikey_usage_counter"`
}

func listUsers(filter bson.M) ([]User, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var users []User
	err = conn.Users().Find(filter).All(&users)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// ListUsers list all users registred in tsuru
func ListUsers() ([]User, error) {
	return listUsers(nil)
}

func ListUsersWithRole(role string) ([]User, error) {
	return listUsers(bson.M{"roles.name": role})
}

func ListUsersWithRolesAndContext(roles []string, context string) ([]User, error) {
	return listUsers(bson.M{"roles": bson.M{"$elemMatch": bson.M{"contextvalue": context, "name": bson.M{"$in": roles}}}})
}

func GetUserByEmail(email string) (*User, error) {
	if !validation.ValidateEmail(email) {
		return nil, &tsuruErrors.ValidationError{Message: "invalid email"}
	}
	var u User
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": email}).One(&u)
	if err != nil {
		return nil, authTypes.ErrUserNotFound
	}
	return &u, nil
}

func (u *User) Create(ctx context.Context) error {
	conn, err := db.Conn()
	if err != nil {
		addr, _ := db.DbConfig("")
		return errors.New(fmt.Sprintf("Failed to connect to MongoDB %q - %s.", addr, err.Error()))
	}
	defer conn.Close()
	if u.Quota.Limit == 0 {
		u.Quota = quota.UnlimitedQuota
		var limit int
		if limit, err = config.GetInt("quota:apps-per-user"); err == nil && limit > -1 {
			u.Quota.Limit = limit
		}
	}
	err = conn.Users().Insert(u)
	if err != nil {
		return err
	}
	err = u.AddRolesForEvent(ctx, permTypes.RoleEventUserCreate, "")
	if err != nil {
		log.Errorf("unable to add default roles during user creation for %q: %s", u.Email, err)
	}
	return nil
}

func (u *User) Delete() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Remove(bson.M{"email": u.Email})
	if err != nil {
		log.Errorf("failed to remove user %q from the database: %s", u.Email, err)
	}

	return nil
}

func (u *User) Update() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Users().Update(bson.M{"email": u.Email}, u)
}

func (u *User) ShowAPIKey() (string, error) {
	if u.APIKey == "" {
		u.RegenerateAPIKey()
	}
	return u.APIKey, u.Update()
}

const keySize = 32

func generateToken(data string, hash crypto.Hash) string {
	var tokenKey [keySize]byte
	n, err := rand.Read(tokenKey[:])
	for n < keySize || err != nil {
		n, err = rand.Read(tokenKey[:])
	}
	h := hash.New()
	h.Write([]byte(data))
	h.Write(tokenKey[:])
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (u *User) RegenerateAPIKey() (string, error) {
	u.APIKey = generateToken(u.Email, crypto.SHA256)
	return u.APIKey, u.Update()
}

func (u *User) Reload() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Users().Find(bson.M{"email": u.Email}).One(u)
}

func expandRolePermissions(ctx context.Context, roleInstances []authTypes.RoleInstance) ([]permission.Permission, error) {
	var permissions []permission.Permission
	roles := make(map[string]*permission.Role)
	for _, roleData := range roleInstances {
		role := roles[roleData.Name]
		if role == nil {
			foundRole, err := permission.FindRole(ctx, roleData.Name)
			if err != nil && err != permTypes.ErrRoleNotFound {
				return nil, err
			}
			role = &foundRole
			roles[roleData.Name] = role
		}
		permissions = append(permissions, role.PermissionsFor(roleData.ContextValue)...)
	}
	return permissions, nil
}

func (u *User) UserGroups() ([]authTypes.Group, error) {
	groupsFilter := []string{}
	if u.Groups != nil {
		groupsFilter = u.Groups
	}
	groups, err := servicemanager.AuthGroup.List(context.TODO(), groupsFilter)
	if err != nil {
		return nil, err
	}
	return groups, nil
}

func (u *User) Permissions(ctx context.Context) ([]permission.Permission, error) {
	groups, err := u.UserGroups()
	if err != nil {
		return nil, err
	}
	allRoles := u.Roles
	for _, group := range groups {
		allRoles = append(allRoles, group.Roles...)
	}
	permissions, err := expandRolePermissions(ctx, allRoles)
	if err != nil {
		return nil, err
	}
	return append([]permission.Permission{{
		Scheme:  permission.PermUser,
		Context: permission.Context(permTypes.CtxUser, u.Email),
	}}, permissions...), nil
}

func (u *User) AddRole(ctx context.Context, roleName string, contextValue string) error {
	_, err := permission.FindRole(ctx, roleName)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Update(bson.M{"email": u.Email}, bson.M{
		"$addToSet": bson.M{
			// Order matters in $addToSet, that's why bson.D is used instead
			// of bson.M.
			"roles": bson.D([]bson.DocElem{
				{Name: "name", Value: roleName},
				{Name: "contextvalue", Value: contextValue},
			}),
		},
	})
	if err != nil {
		return err
	}
	return u.Reload()
}

func UpdateRoleFromAllUsers(ctx context.Context, roleName, newRoleName, permissionCtx, desc string) error {
	role, err := permission.FindRole(ctx, roleName)
	if err != nil {
		return permTypes.ErrRoleNotFound
	}
	if permissionCtx != "" {
		role.ContextType, err = permission.ParseContext(permissionCtx)
		if err != nil {
			return err
		}
	}
	if desc != "" {
		role.Description = desc
	}
	if (newRoleName == "") || (role.Name == newRoleName) {
		return role.Update(ctx)
	}
	role.Name = newRoleName
	err = role.Add(ctx)
	if err != nil {
		return err
	}
	usersWithRole, err := ListUsersWithRole(roleName)
	if err != nil {
		errDtr := permission.DestroyRole(ctx, role.Name)
		if errDtr != nil {
			return tsuruErrors.NewMultiError(err, errDtr)
		}
		return err
	}
	for _, user := range usersWithRole {
		errAddRole := user.AddRole(ctx, role.Name, string(role.ContextType))
		if errAddRole != nil {
			errDtr := permission.DestroyRole(ctx, role.Name)
			if errDtr != nil {
				return tsuruErrors.NewMultiError(errAddRole, errDtr)
			}
			errRmv := RemoveRoleFromAllUsers(roleName)
			if errRmv != nil {
				return tsuruErrors.NewMultiError(errAddRole, errRmv)
			}
			return errAddRole
		}
	}
	err = permission.DestroyRole(ctx, roleName)
	if err != nil {
		return err
	}
	return RemoveRoleFromAllUsers(roleName)
}

func RemoveRoleFromAllUsers(roleName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Users().UpdateAll(bson.M{"roles.name": roleName}, bson.M{
		"$pull": bson.M{
			"roles": bson.M{"name": roleName},
		},
	})
	return err
}

func (u *User) RemoveRole(roleName string, contextValue string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Update(bson.M{"email": u.Email}, bson.M{
		"$pull": bson.M{
			"roles": bson.D([]bson.DocElem{
				{Name: "name", Value: roleName},
				{Name: "contextvalue", Value: contextValue},
			}),
		},
	})
	if err != nil {
		return err
	}
	return u.Reload()
}

func (u *User) AddRolesForEvent(ctx context.Context, roleEvent *permTypes.RoleEvent, contextValue string) error {
	roles, err := permission.ListRolesForEvent(ctx, roleEvent)
	if err != nil {
		return errors.Wrap(err, "unable to list roles")
	}
	for _, r := range roles {
		err = u.AddRole(ctx, r.Name, contextValue)
		if err != nil {
			return errors.Wrap(err, "unable to add role")
		}
	}
	return nil
}

func (u *User) GetName() string {
	return u.Email
}
