// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto"
	"crypto/rand"
	_ "crypto/sha256"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrUserNotFound = stderrors.New("user not found")
	ErrInvalidKey   = stderrors.New("invalid key")
	ErrKeyDisabled  = stderrors.New("key management is disabled")
)

type RoleInstance struct {
	Name         string
	ContextValue string
}

type User struct {
	quota.Quota
	Email    string
	Password string
	APIKey   string
	Roles    []RoleInstance `bson:",omitempty"`
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

func ListUsersWithPermissions(wantedPerms ...permission.Permission) ([]User, error) {
	allUsers, err := ListUsers()
	if err != nil {
		return nil, err
	}
	var filteredUsers []User
	// TODO(cezarsa): Too slow! Think about faster implementation in the future.
usersLoop:
	for _, u := range allUsers {
		perms, err := u.Permissions()
		if err != nil {
			return nil, err
		}
		for _, p := range wantedPerms {
			if permission.CheckFromPermList(perms, p.Scheme, p.Context) {
				filteredUsers = append(filteredUsers, u)
				continue usersLoop
			}
		}
	}
	return filteredUsers, nil
}

func GetUserByEmail(email string) (*User, error) {
	if !validation.ValidateEmail(email) {
		return nil, &errors.ValidationError{Message: "invalid email"}
	}
	var u User
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": email}).One(&u)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

func (u *User) Create() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if u.Quota.Limit == 0 {
		u.Quota = quota.Unlimited
		var limit int
		if limit, err = config.GetInt("quota:apps-per-user"); err == nil && limit > -1 {
			u.Quota.Limit = limit
		}
	}
	err = conn.Users().Insert(u)
	if err != nil {
		return err
	}
	err = u.createOnRepositoryManager()
	if err != nil {
		u.Delete()
		return err
	}
	err = u.AddRolesForEvent(permission.RoleEventUserCreate, "")
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
	err = repository.Manager().RemoveUser(u.Email)
	if err != nil {
		log.Errorf("failed to remove user %q from the repository manager: %s", u.Email, err)
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

func (u *User) AddKey(key repository.Key, force bool) error {
	if mngr, ok := repository.Manager().(repository.KeyRepositoryManager); ok {
		if key.Name == "" {
			return ErrInvalidKey
		}
		err := mngr.AddKey(u.Email, key)
		if err == repository.ErrKeyAlreadyExists && force {
			return mngr.UpdateKey(u.Email, key)
		}
		return err
	}
	return ErrKeyDisabled
}

func (u *User) RemoveKey(key repository.Key) error {
	if mngr, ok := repository.Manager().(repository.KeyRepositoryManager); ok {
		return mngr.RemoveKey(u.Email, key)
	}
	return ErrKeyDisabled
}

func (u *User) ListKeys() (map[string]string, error) {
	if mngr, ok := repository.Manager().(repository.KeyRepositoryManager); ok {
		keys, err := mngr.ListKeys(u.Email)
		if err != nil {
			return nil, err
		}
		keysMap := make(map[string]string, len(keys))
		for _, key := range keys {
			keysMap[key.Name] = key.Body
		}
		return keysMap, nil
	}
	return nil, ErrKeyDisabled
}

func (u *User) createOnRepositoryManager() error {
	err := repository.Manager().CreateUser(u.Email)
	if err != nil {
		return err
	}
	return nil
}

func (u *User) ShowAPIKey() (string, error) {
	if u.APIKey == "" {
		u.RegenerateAPIKey()
	}
	return u.APIKey, u.Update()
}

func (u *User) RegenerateAPIKey() (string, error) {
	random_byte := make([]byte, 32)
	_, err := rand.Read(random_byte)
	if err != nil {
		return "", err
	}
	h := crypto.SHA256.New()
	h.Write([]byte(u.Email))
	h.Write(random_byte)
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	u.APIKey = fmt.Sprintf("%x", h.Sum(nil))
	return u.APIKey, u.Update()
}

func (u *User) reload() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Users().Find(bson.M{"email": u.Email}).One(u)
}

func (u *User) Permissions() ([]permission.Permission, error) {
	var permissions []permission.Permission
	roles := make(map[string]*permission.Role)
	for _, roleData := range u.Roles {
		role := roles[roleData.Name]
		if role == nil {
			foundRole, err := permission.FindRole(roleData.Name)
			if err != nil {
				return nil, err
			}
			role = &foundRole
			roles[roleData.Name] = role
		}
		permissions = append(permissions, role.PermissionsFor(roleData.ContextValue)...)
	}
	return permissions, nil
}

func (u *User) AddRole(roleName string, contextValue string) error {
	_, err := permission.FindRole(roleName)
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
	return u.reload()
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
	return u.reload()
}

func (u *User) AddRolesForEvent(roleEvent *permission.RoleEvent, contextValue string) error {
	roles, err := permission.ListRolesForEvent(roleEvent)
	if err != nil {
		return fmt.Errorf("unable to list roles: %s", err)
	}
	for _, r := range roles {
		err = u.AddRole(r.Name, contextValue)
		if err != nil {
			return fmt.Errorf("unable to add role: %s", err)
		}
	}
	return nil
}
