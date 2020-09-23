// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"sort"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestCreateUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	var result User
	collection := s.conn.Users()
	err = collection.Find(bson.M{"email": u.Email}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Email, check.Equals, u.Email)
	c.Assert(repositorytest.Users(), check.DeepEquals, []string{u.Email})
}

func (s *S) TestCreateUserReturnsErrorWhenTryingToCreateAUserWithDuplicatedEmail(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	err = u.Create()
	c.Assert(err, check.NotNil)
}

func (s *S) TestCreateUserWhenMongoDbIsDown(c *check.C) {
	oldURL, _ := config.Get("database:url")
	config.Unset("database:url")
	defer config.Set("database:url", oldURL)
	config.Set("database:url", "invalid")
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to connect to MongoDB \"invalid\" - no reachable servers.")
}

func (s *S) TestGetUserByEmail(c *check.C) {
	u := User{Email: "wolmverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	u2, err := GetUserByEmail(u.Email)
	c.Assert(err, check.IsNil)
	c.Check(u2.Email, check.Equals, u.Email)
	c.Check(u2.Password, check.Equals, u.Password)
}

func (s *S) TestGetUserByEmailReturnsErrorWhenNoUserIsFound(c *check.C) {
	u, err := GetUserByEmail("unknown@globo.com")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *S) TestGetUserByEmailWithInvalidEmail(c *check.C) {
	u, err := GetUserByEmail("unknown")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Message, check.Equals, "invalid email")
}

func (s *S) TestUpdateUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	u.Password = "1234"
	err = u.Update()
	c.Assert(err, check.IsNil)
	u2, err := GetUserByEmail("wolverine@xmen.com")
	c.Assert(err, check.IsNil)
	c.Assert(u2.Password, check.Equals, "1234")
}

func (s *S) TestDeleteUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	err = u.Delete()
	c.Assert(err, check.IsNil)
	user, err := GetUserByEmail(u.Email)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
	c.Assert(user, check.IsNil)
	c.Assert(repositorytest.Users(), check.HasLen, 0)
}

func (s *S) TestAddKeyAddsAKeyToTheUser(c *check.C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Name: "some-key", Body: "my-key"}
	err = u.AddKey(key, false)
	c.Assert(err, check.IsNil)
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(context.TODO(), u.Email)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.DeepEquals, []repository.Key{key})
}

func (s *S) TestAddKeyEmptyName(c *check.C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Body: "my-key"}
	err = u.AddKey(key, false)
	c.Assert(err, check.Equals, authTypes.ErrInvalidKey)
}

func (s *S) TestAddDuplicatedKey(c *check.C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Name: "my-key", Body: "other-key"}
	err = u.AddKey(key, false)
	c.Assert(err, check.IsNil)
	err = u.AddKey(key, false)
	c.Assert(err, check.Equals, repository.ErrKeyAlreadyExists)
}

func (s *S) TestAddKeyDuplicatedForce(c *check.C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Name: "some-key", Body: "my-key"}
	err = u.AddKey(key, false)
	c.Assert(err, check.IsNil)
	newKey := repository.Key{Name: "some-key", Body: "my-new-key"}
	err = u.AddKey(newKey, true)
	c.Assert(err, check.IsNil)
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(context.TODO(), u.Email)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.DeepEquals, []repository.Key{newKey})
}

func (s *S) TestAddKeyDisabled(c *check.C) {
	config.Set("repo-manager", "none")
	defer config.Set("repo-manager", "fake")
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Name: "my-key", Body: "other-key"}
	err = u.AddKey(key, false)
	c.Assert(err, check.Equals, authTypes.ErrKeyDisabled)
}

func (s *S) TestRemoveKeyRemovesAKeyFromTheUser(c *check.C) {
	key := repository.Key{Body: "my-key", Name: "the-key"}
	u := &User{Email: "shineon@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	err = repository.Manager().(repository.KeyRepositoryManager).AddKey(context.TODO(), u.Email, key)
	c.Assert(err, check.IsNil)
	err = u.RemoveKey(repository.Key{Name: "the-key"})
	c.Assert(err, check.IsNil)
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(context.TODO(), u.Email)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.HasLen, 0)
}

func (s *S) TestRemoveUnknownKey(c *check.C) {
	u := &User{Email: "shine@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	err = u.RemoveKey(repository.Key{Body: "my-key"})
	c.Assert(err, check.Equals, repository.ErrKeyNotFound)
}

func (s *S) TestRemoveKeyDisabled(c *check.C) {
	config.Set("repo-manager", "none")
	defer config.Set("repo-manager", "fake")
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Name: "my-key", Body: "other-key"}
	err = u.RemoveKey(key)
	c.Assert(err, check.Equals, authTypes.ErrKeyDisabled)
}

func (s *S) TestListKeysShouldGetKeysFromTheRepositoryManager(c *check.C) {
	u := User{
		Email:    "wolverine@xmen.com",
		Password: "123456",
	}
	newKeys := []repository.Key{{Name: "key1", Body: "superkey"}, {Name: "key2", Body: "hiperkey"}}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	repository.Manager().(repository.KeyRepositoryManager).AddKey(context.TODO(), u.Email, newKeys[0])
	repository.Manager().(repository.KeyRepositoryManager).AddKey(context.TODO(), u.Email, newKeys[1])
	keys, err := u.ListKeys()
	c.Assert(err, check.IsNil)
	expected := map[string]string{"key1": "superkey", "key2": "hiperkey"}
	c.Assert(keys, check.DeepEquals, expected)
}

func (s *S) TestListKeysRepositoryManagerFailure(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	err = repository.Manager().RemoveUser(context.TODO(), u.Email)
	c.Assert(err, check.IsNil)
	keys, err := u.ListKeys()
	c.Assert(keys, check.HasLen, 0)
	c.Assert(err.Error(), check.Equals, "user not found")
}

func (s *S) TestListKeysDisabled(c *check.C) {
	config.Set("repo-manager", "none")
	defer config.Set("repo-manager", "fake")
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	keys, err := u.ListKeys()
	c.Assert(err, check.Equals, authTypes.ErrKeyDisabled)
	c.Assert(keys, check.IsNil)
}

func (s *S) TestShowAPIKeyWhenAPITokenAlreadyExists(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		APIKey:   "1ioudh8ydb2idn1ehnpoqwjmhdjqwz12po1",
	}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	API_Token, err := u.ShowAPIKey()
	c.Assert(API_Token, check.Equals, u.APIKey)
	c.Assert(err, check.IsNil)
}

func (s *S) TestShowAPIKeyWhenAPITokenNotExists(c *check.C) {
	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	API_Token, err := u.ShowAPIKey()
	c.Assert(API_Token, check.Equals, u.APIKey)
	c.Assert(err, check.IsNil)
}

func (s *S) TestListAllUsers(c *check.C) {
	users, err := ListUsers()
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
	_, err := permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("r2", "app", "")
	c.Assert(err, check.IsNil)
	u := User{Email: "me@tsuru.com", Password: "123"}
	err = u.Create()
	c.Assert(err, check.IsNil)
	err = u.AddRole("r1", "c1")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r1", "c2")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r2", "x")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r2", "x")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r3", "a")
	c.Assert(err, check.Equals, permTypes.ErrRoleNotFound)
	expected := []authTypes.RoleInstance{
		{Name: "r1", ContextValue: "c1"},
		{Name: "r1", ContextValue: "c2"},
		{Name: "r2", ContextValue: "x"},
	}
	sort.Sort(roleInstanceList(expected))
	sort.Sort(roleInstanceList(u.Roles))
	c.Assert(u.Roles, check.DeepEquals, expected)
	uDB, err := GetUserByEmail("me@tsuru.com")
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
	err := u.Create()
	c.Assert(err, check.IsNil)
	err = u.RemoveRole("r1", "c2")
	c.Assert(err, check.IsNil)
	err = u.RemoveRole("r1", "c2")
	c.Assert(err, check.IsNil)
	expected := []authTypes.RoleInstance{
		{Name: "r1", ContextValue: "c1"},
		{Name: "r2", ContextValue: "x"},
	}
	sort.Sort(roleInstanceList(expected))
	sort.Sort(roleInstanceList(u.Roles))
	c.Assert(u.Roles, check.DeepEquals, expected)
	uDB, err := GetUserByEmail("me@tsuru.com")
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
	err := u.Create()
	c.Assert(err, check.IsNil)
	err = RemoveRoleFromAllUsers("r1")
	c.Assert(err, check.IsNil)
	expected := []authTypes.RoleInstance{
		{Name: "r2", ContextValue: "x"},
	}
	sort.Sort(roleInstanceList(expected))
	uDB, err := GetUserByEmail("me@tsuru.com")
	c.Assert(err, check.IsNil)
	sort.Sort(roleInstanceList(u.Roles))
	c.Assert(uDB.Roles, check.DeepEquals, expected)
	sort.Sort(roleInstanceList(uDB.Roles))
	c.Assert(uDB.Roles, check.DeepEquals, expected)
}

func (s *S) TestUserPermissions(c *check.C) {

	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)

	perms, err := u.Permissions()
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermUser, Context: permission.Context(permTypes.CtxUser, u.Email)},
	})

	r1, err := permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r1", "myapp")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r1", "myapp2")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole("g1", "r1", "myapp3")
	c.Assert(err, check.IsNil)

	perms, err = u.Permissions()
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
	err := u.Create()
	c.Assert(err, check.IsNil)

	r1, err := permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r1", "myapp")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r1", "myapp2")
	c.Assert(err, check.IsNil)

	err = servicemanager.AuthGroup.AddRole("g2", "r1", "myapp3")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole("g3", "r1", "myapp4")
	c.Assert(err, check.IsNil)

	perms, err := u.Permissions()
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
	role, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	u := User{Email: "me@tsuru.com", Password: "123"}
	err = u.Create()
	c.Assert(err, check.IsNil)
	err = u.AddRole(role.Name, "team")
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Roles().RemoveId(role.Name)
	c.Assert(err, check.IsNil)
	perms, err := u.Permissions()
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermUser, Context: permission.Context(permTypes.CtxUser, u.Email)},
	})
	r1, err := permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r1", "myapp")
	c.Assert(err, check.IsNil)
	err = u.AddRole("r1", "myapp2")
	c.Assert(err, check.IsNil)
	perms, err = u.Permissions()
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermUser, Context: permission.Context(permTypes.CtxUser, u.Email)},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp2")},
		{Scheme: permission.PermAppUpdateEnv, Context: permission.Context(permTypes.CtxApp, "myapp2")},
	})
}

func (s *S) TestListUsersWithPermissions(c *check.C) {
	u1 := User{Email: "me1@tsuru.com", Password: "123"}
	err := u1.Create()
	c.Assert(err, check.IsNil)
	u2 := User{Email: "me2@tsuru.com", Password: "123"}
	err = u2.Create()
	c.Assert(err, check.IsNil)
	r1, err := permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u1.AddRole("r1", "myapp1")
	c.Assert(err, check.IsNil)
	err = u2.AddRole("r1", "myapp2")
	c.Assert(err, check.IsNil)
	users, err := ListUsersWithPermissions(permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxApp, "myapp1"),
	})
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
	c.Assert(users[0].Email, check.Equals, u1.Email)
	users, err = ListUsersWithPermissions(permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxApp, "myapp2"),
	})
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
	c.Assert(users[0].Email, check.Equals, u2.Email)
}

func (s *S) TestAddRolesForEvent(c *check.C) {
	r1, err := permission.NewRole("r1", "team", "")
	c.Assert(err, check.IsNil)
	err = r1.AddEvent(permTypes.RoleEventTeamCreate.String())
	c.Assert(err, check.IsNil)
	u1 := User{Email: "me1@tsuru.com", Password: "123"}
	err = u1.Create()
	c.Assert(err, check.IsNil)
	err = u1.AddRolesForEvent(permTypes.RoleEventTeamCreate, "team1")
	c.Assert(err, check.IsNil)
	u, err := GetUserByEmail(u1.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u.Roles, check.DeepEquals, []authTypes.RoleInstance{{Name: "r1", ContextValue: "team1"}})
}

func (s *S) TestUpdateRoleFromAllUsers(c *check.C) {
	u1 := User{Email: "me1@tsuru.com", Password: "123"}
	err := u1.Create()
	c.Assert(err, check.IsNil)
	u2 := User{Email: "me2@tsuru.com", Password: "123"}
	err = u2.Create()
	c.Assert(err, check.IsNil)
	r1, err := permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u1.AddRole("r1", "myapp1")
	c.Assert(err, check.IsNil)
	err = u2.AddRole("r1", "myapp2")
	c.Assert(err, check.IsNil)
	err = UpdateRoleFromAllUsers("r1", "r2", "team", "")
	c.Assert(err, check.IsNil)
	r, err := permission.FindRole("r2")
	c.Assert(err, check.IsNil)
	c.Assert(r.Name, check.Equals, "r2")
	c.Assert(string(r.ContextType), check.Equals, "team")
	err = UpdateRoleFromAllUsers("r1", "", "app", "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "role not found")
}

func (s *S) TestUpdateRoleFromAllUsersWithSameNameAndDifferentDescriptions(c *check.C) {
	u1 := User{Email: "me1@tsuru.com", Password: "123"}
	err := u1.Create()
	c.Assert(err, check.IsNil)
	u2 := User{Email: "me2@tsuru.com", Password: "123"}
	err = u2.Create()
	c.Assert(err, check.IsNil)
	r1, err := permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("app.update.env", "app.deploy")
	c.Assert(err, check.IsNil)
	err = u1.AddRole("r1", "myapp1")
	c.Assert(err, check.IsNil)
	err = u2.AddRole("r1", "myapp2")
	c.Assert(err, check.IsNil)
	err = UpdateRoleFromAllUsers("r1", "", "team", "some description")
	c.Assert(err, check.IsNil)
	r, err := permission.FindRole("r1")
	c.Assert(err, check.IsNil)
	c.Assert(r.Name, check.Equals, "r1")
	c.Assert(string(r.ContextType), check.Equals, "team")
	c.Assert(r.Description, check.Equals, "some description")
}
