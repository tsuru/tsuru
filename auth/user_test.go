// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"sort"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestCreateUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete(context.TODO())
	var result User
	collection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)
	err = collection.FindOne(context.TODO(), mongoBSON.M{"email": u.Email}).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Email, check.Equals, u.Email)
}

func (s *S) TestCreateUserReturnsErrorWhenTryingToCreateAUserWithDuplicatedEmail(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete(context.TODO())
	err = u.Create(context.TODO())
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetUserByEmail(c *check.C) {
	u := User{Email: "wolmverine@xmen.com", Password: "123456"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete(context.TODO())
	u2, err := GetUserByEmail(context.TODO(), u.Email)
	c.Assert(err, check.IsNil)
	c.Check(u2.Email, check.Equals, u.Email)
	c.Check(u2.Password, check.Equals, u.Password)
}

func (s *S) TestGetUserByEmailReturnsErrorWhenNoUserIsFound(c *check.C) {
	u, err := GetUserByEmail(context.TODO(), "unknown@globo.com")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *S) TestGetUserByEmailWithInvalidEmail(c *check.C) {
	u, err := GetUserByEmail(context.TODO(), "unknown")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Message, check.Equals, "invalid email")
}

func (s *S) TestUpdateUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete(context.TODO())
	u.Password = "1234"
	err = u.Update(context.TODO())
	c.Assert(err, check.IsNil)
	u2, err := GetUserByEmail(context.TODO(), "wolverine@xmen.com")
	c.Assert(err, check.IsNil)
	c.Assert(u2.Password, check.Equals, "1234")
}

func (s *S) TestDeleteUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	err = u.Delete(context.TODO())
	c.Assert(err, check.IsNil)
	user, err := GetUserByEmail(context.TODO(), u.Email)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
	c.Assert(user, check.IsNil)
}

func (s *S) TestShowAPIKeyWhenAPITokenAlreadyExists(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		APIKey:   "1ioudh8ydb2idn1ehnpoqwjmhdjqwz12po1",
	}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete(context.TODO())
	API_Token, err := u.ShowAPIKey(context.TODO())
	c.Assert(API_Token, check.Equals, u.APIKey)
	c.Assert(err, check.IsNil)
}

func (s *S) TestShowAPIKeyWhenAPITokenNotExists(c *check.C) {
	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete(context.TODO())
	API_Token, err := u.ShowAPIKey(context.TODO())
	c.Assert(API_Token, check.Equals, u.APIKey)
	c.Assert(err, check.IsNil)
}

func (s *S) TestListAllUsers(c *check.C) {
	users, err := ListUsers(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
}

type roleInstanceList []authTypes.RoleInstance

func (l roleInstanceList) Len() int      { return len(l) }
func (l roleInstanceList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l roleInstanceList) Less(i, j int) bool {
	if l[i].Name < l[j].Name {
		return true
	} else if l[i].Name > l[j].Name {
		return false
	} else {
		return l[i].ContextValue < l[j].ContextValue
	}
}

func (s *S) TestUserAddRole(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "r1", "app", "")
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole(context.TODO(), "r2", "app", "")
	c.Assert(err, check.IsNil)
	u := User{Email: "me@tsuru.com", Password: "123"}
	err = u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r1", "c1")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r1", "c2")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r2", "x")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r2", "x")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r3", "a")
	c.Assert(err, check.Equals, permTypes.ErrRoleNotFound)
	expected := []authTypes.RoleInstance{
		{Name: "r1", ContextValue: "c1"},
		{Name: "r1", ContextValue: "c2"},
		{Name: "r2", ContextValue: "x"},
	}
	sort.Sort(roleInstanceList(expected))
	sort.Sort(roleInstanceList(u.Roles))
	c.Assert(u.Roles, check.DeepEquals, expected)
	uDB, err := GetUserByEmail(context.TODO(), "me@tsuru.com")
	c.Assert(err, check.IsNil)
	sort.Sort(roleInstanceList(uDB.Roles))
	c.Assert(uDB.Roles, check.DeepEquals, expected)
}

func (s *S) TestUserRemoveRole(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		Roles: []authTypes.RoleInstance{
			{Name: "r1", ContextValue: "c1"},
			{Name: "r1", ContextValue: "c2"},
			{Name: "r2", ContextValue: "x"},
		},
	}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	err = u.RemoveRole(context.TODO(), "r1", "c2")
	c.Assert(err, check.IsNil)
	err = u.RemoveRole(context.TODO(), "r1", "c2")
	c.Assert(err, check.IsNil)
	expected := []authTypes.RoleInstance{
		{Name: "r1", ContextValue: "c1"},
		{Name: "r2", ContextValue: "x"},
	}
	sort.Sort(roleInstanceList(expected))
	sort.Sort(roleInstanceList(u.Roles))
	c.Assert(u.Roles, check.DeepEquals, expected)
	uDB, err := GetUserByEmail(context.TODO(), "me@tsuru.com")
	c.Assert(err, check.IsNil)
	sort.Sort(roleInstanceList(uDB.Roles))
	c.Assert(uDB.Roles, check.DeepEquals, expected)
}

func (s *S) TestRemoveRoleFromAllUsers(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		Roles: []authTypes.RoleInstance{
			{Name: "r1", ContextValue: "c1"},
			{Name: "r1", ContextValue: "c2"},
			{Name: "r2", ContextValue: "x"},
		},
	}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	err = RemoveRoleFromAllUsers(context.TODO(), "r1")
	c.Assert(err, check.IsNil)
	expected := []authTypes.RoleInstance{
		{Name: "r2", ContextValue: "x"},
	}
	sort.Sort(roleInstanceList(expected))
	uDB, err := GetUserByEmail(context.TODO(), "me@tsuru.com")
	c.Assert(err, check.IsNil)
	sort.Sort(roleInstanceList(u.Roles))
	c.Assert(uDB.Roles, check.DeepEquals, expected)
	sort.Sort(roleInstanceList(uDB.Roles))
	c.Assert(uDB.Roles, check.DeepEquals, expected)
}

func (s *S) TestUserPermissions(c *check.C) {

	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)

	perms, err := u.Permissions(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermUser, Context: permission.Context(permTypes.CtxUser, u.Email)},
	})

	r1, err := permission.NewRole(context.TODO(), "r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions(context.TODO(), "app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r1", "myapp")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r1", "myapp2")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole(context.TODO(), "g1", "r1", "myapp3")
	c.Assert(err, check.IsNil)

	perms, err = u.Permissions(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermUser, Context: permission.Context(permTypes.CtxUser, u.Email)},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp2")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp2")},
	})
}

func (s *S) TestUserPermissionsIncludeGroups(c *check.C) {
	u := User{Email: "me@tsuru.com", Password: "123", Groups: []string{"g1", "g2"}}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)

	r1, err := permission.NewRole(context.TODO(), "r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions(context.TODO(), "app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r1", "myapp")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r1", "myapp2")
	c.Assert(err, check.IsNil)

	err = servicemanager.AuthGroup.AddRole(context.TODO(), "g2", "r1", "myapp3")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole(context.TODO(), "g3", "r1", "myapp4")
	c.Assert(err, check.IsNil)

	perms, err := u.Permissions(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermUser, Context: permission.Context(permTypes.CtxUser, u.Email)},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp2")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp2")},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp3")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp3")},
	})
}

func (s *S) TestUserPermissionsWithRemovedRole(c *check.C) {
	role, err := permission.NewRole(context.TODO(), "test", "team", "")
	c.Assert(err, check.IsNil)
	u := User{Email: "me@tsuru.com", Password: "123"}
	err = u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), role.Name, "team")
	c.Assert(err, check.IsNil)
	rolesCollection, err := storagev2.RolesCollection()
	c.Assert(err, check.IsNil)
	_, err = rolesCollection.DeleteOne(context.TODO(), mongoBSON.M{"_id": role.Name})
	c.Assert(err, check.IsNil)
	perms, err := u.Permissions(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermUser, Context: permission.Context(permTypes.CtxUser, u.Email)},
	})
	r1, err := permission.NewRole(context.TODO(), "r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions(context.TODO(), "app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r1", "myapp")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "r1", "myapp2")
	c.Assert(err, check.IsNil)
	perms, err = u.Permissions(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermUser, Context: permission.Context(permTypes.CtxUser, u.Email)},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp2")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp2")},
	})
}

func (s *S) TestAddRolesForEvent(c *check.C) {
	r1, err := permission.NewRole(context.TODO(), "r1", "team", "")
	c.Assert(err, check.IsNil)
	err = r1.AddEvent(context.TODO(), permTypes.RoleEventTeamCreate.String())
	c.Assert(err, check.IsNil)
	u1 := User{Email: "me1@tsuru.com", Password: "123"}
	err = u1.Create(context.TODO())
	c.Assert(err, check.IsNil)
	err = u1.AddRolesForEvent(context.TODO(), permTypes.RoleEventTeamCreate, "team1")
	c.Assert(err, check.IsNil)
	u, err := GetUserByEmail(context.TODO(), u1.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u.Roles, check.DeepEquals, []authTypes.RoleInstance{{Name: "r1", ContextValue: "team1"}})
}

func (s *S) TestUpdateRoleFromAllUsers(c *check.C) {
	u1 := User{Email: "me1@tsuru.com", Password: "123"}
	err := u1.Create(context.TODO())
	c.Assert(err, check.IsNil)
	u2 := User{Email: "me2@tsuru.com", Password: "123"}
	err = u2.Create(context.TODO())
	c.Assert(err, check.IsNil)
	r1, err := permission.NewRole(context.TODO(), "r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions(context.TODO(), "app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u1.AddRole(context.TODO(), "r1", "myapp1")
	c.Assert(err, check.IsNil)
	err = u2.AddRole(context.TODO(), "r1", "myapp2")
	c.Assert(err, check.IsNil)
	err = UpdateRoleFromAllUsers(context.TODO(), "r1", "r2", "team", "")
	c.Assert(err, check.IsNil)
	r, err := permission.FindRole(context.TODO(), "r2")
	c.Assert(err, check.IsNil)
	c.Assert(r.Name, check.Equals, "r2")
	c.Assert(string(r.ContextType), check.Equals, "team")
	err = UpdateRoleFromAllUsers(context.TODO(), "r1", "", "app", "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "role not found")
}

func (s *S) TestUpdateRoleFromAllUsersWithSameNameAndDifferentDescriptions(c *check.C) {
	u1 := User{Email: "me1@tsuru.com", Password: "123"}
	err := u1.Create(context.TODO())
	c.Assert(err, check.IsNil)
	u2 := User{Email: "me2@tsuru.com", Password: "123"}
	err = u2.Create(context.TODO())
	c.Assert(err, check.IsNil)
	r1, err := permission.NewRole(context.TODO(), "r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions(context.TODO(), "app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u1.AddRole(context.TODO(), "r1", "myapp1")
	c.Assert(err, check.IsNil)
	err = u2.AddRole(context.TODO(), "r1", "myapp2")
	c.Assert(err, check.IsNil)
	err = UpdateRoleFromAllUsers(context.TODO(), "r1", "", "team", "some description")
	c.Assert(err, check.IsNil)
	r, err := permission.FindRole(context.TODO(), "r1")
	c.Assert(err, check.IsNil)
	c.Assert(r.Name, check.Equals, "r1")
	c.Assert(string(r.ContextType), check.Equals, "team")
	c.Assert(r.Description, check.Equals, "some description")
}
