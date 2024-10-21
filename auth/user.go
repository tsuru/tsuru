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

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/validation"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
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

func listUsers(ctx context.Context, filter mongoBSON.M) ([]User, error) {
	if filter == nil {
		filter = mongoBSON.M{}
	}
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return nil, err
	}
	var users []User
	cursor, err := usersCollection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &users)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// ListUsers list all users registred in tsuru
func ListUsers(ctx context.Context) ([]User, error) {
	return listUsers(ctx, nil)
}

func ListUsersWithRole(ctx context.Context, role string) ([]User, error) {
	return listUsers(ctx, mongoBSON.M{"roles.name": role})
}

func ListUsersWithRolesAndContext(ctx context.Context, roles []string, context string) ([]User, error) {
	return listUsers(ctx, mongoBSON.M{"roles": mongoBSON.M{"$elemMatch": mongoBSON.M{"contextvalue": context, "name": mongoBSON.M{"$in": roles}}}})
}

func GetUserByEmail(ctx context.Context, email string) (*User, error) {
	if !validation.ValidateEmail(email) {
		return nil, &tsuruErrors.ValidationError{Message: "invalid email"}
	}
	var u User
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return nil, err
	}
	err = usersCollection.FindOne(ctx, mongoBSON.M{"email": email}).Decode(&u)
	if err != nil {
		return nil, authTypes.ErrUserNotFound
	}
	return &u, nil
}

func (u *User) Create(ctx context.Context) error {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}
	if u.Quota.Limit == 0 {
		u.Quota = quota.UnlimitedQuota
		var limit int
		if limit, err = config.GetInt("quota:apps-per-user"); err == nil && limit > -1 {
			u.Quota.Limit = limit
		}
	}
	_, err = usersCollection.InsertOne(ctx, u)
	if err != nil {
		return err
	}
	err = u.AddRolesForEvent(ctx, permTypes.RoleEventUserCreate, "")
	if err != nil {
		log.Errorf("unable to add default roles during user creation for %q: %s", u.Email, err)
	}
	return nil
}

func (u *User) Delete(ctx context.Context) error {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}
	_, err = usersCollection.DeleteOne(ctx, mongoBSON.M{"email": u.Email})
	if err != nil {
		log.Errorf("failed to remove user %q from the database: %s", u.Email, err)
	}

	return nil
}

func (u *User) Update(ctx context.Context) error {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}
	_, err = usersCollection.ReplaceOne(ctx, mongoBSON.M{"email": u.Email}, u)
	return err
}

func (u *User) ShowAPIKey(ctx context.Context) (string, error) {
	if u.APIKey == "" {
		u.RegenerateAPIKey(ctx)
	}
	return u.APIKey, u.Update(ctx)
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

func (u *User) RegenerateAPIKey(ctx context.Context) (string, error) {
	u.APIKey = generateToken(u.Email, crypto.SHA256)
	return u.APIKey, u.Update(ctx)
}

func (u *User) reload(ctx context.Context) error {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}
	return usersCollection.FindOne(ctx, mongoBSON.M{"email": u.Email}).Decode(u)
}

func expandRolePermissions(ctx context.Context, roleInstances []authTypes.RoleInstance) ([]permTypes.Permission, error) {
	var permissions []permTypes.Permission
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

func (u *User) Permissions(ctx context.Context) ([]permTypes.Permission, error) {
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
	return append([]permTypes.Permission{{
		Scheme:  permission.PermUser,
		Context: permission.Context(permTypes.CtxUser, u.Email),
	}}, permissions...), nil
}

func (u *User) AddRole(ctx context.Context, roleName string, contextValue string) error {
	_, err := permission.FindRole(ctx, roleName)
	if err != nil {
		return err
	}
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}
	_, err = usersCollection.UpdateOne(ctx, mongoBSON.M{"email": u.Email}, mongoBSON.M{
		"$addToSet": mongoBSON.M{
			// Order matters in $addToSet, that's why bson.D is used instead
			// of bson.M.
			"roles": mongoBSON.D([]mongoBSON.E{
				{Key: "name", Value: roleName},
				{Key: "contextvalue", Value: contextValue},
			}),
		},
	})
	if err != nil {
		return err
	}
	return u.reload(ctx)
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
	usersWithRole, err := ListUsersWithRole(ctx, roleName)
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
			errRmv := RemoveRoleFromAllUsers(ctx, roleName)
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
	return RemoveRoleFromAllUsers(ctx, roleName)
}

func RemoveRoleFromAllUsers(ctx context.Context, roleName string) error {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}
	_, err = usersCollection.UpdateMany(ctx, mongoBSON.M{"roles.name": roleName}, mongoBSON.M{
		"$pull": mongoBSON.M{
			"roles": mongoBSON.M{"name": roleName},
		},
	})
	return err
}

func (u *User) RemoveRole(ctx context.Context, roleName string, contextValue string) error {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return err
	}
	_, err = usersCollection.UpdateOne(ctx, mongoBSON.M{"email": u.Email}, mongoBSON.M{
		"$pull": mongoBSON.M{
			"roles": mongoBSON.D([]mongoBSON.E{
				{Key: "name", Value: roleName},
				{Key: "contextvalue", Value: contextValue},
			}),
		},
	})
	if err != nil {
		return err
	}
	return u.reload(ctx)
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
