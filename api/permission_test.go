// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	routerTypes "github.com/tsuru/tsuru/types/router"
	"github.com/tsuru/tsuru/types/volume"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestAddRole(c *check.C) {
	ctx := context.TODO()

	rolesCollection, err := storagev2.RolesCollection()
	c.Assert(err, check.IsNil)

	_, err = rolesCollection.DeleteMany(ctx, mongoBSON.M{})
	c.Assert(err, check.IsNil)

	role := bytes.NewBufferString("name=test&context=global")
	req, err := http.NewRequest(http.MethodPost, "/roles", role)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleCreate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	roles, err := permission.ListRoles(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(roles, check.HasLen, 2)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "test"},
			{"name": "context", "value": "global"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddRoleUnauthorized(c *check.C) {
	role := bytes.NewBufferString("name=test&context=global")
	req, err := http.NewRequest(http.MethodPost, "/roles", role)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAddRoleInvalidName(c *check.C) {
	role := bytes.NewBufferString("name=&context=global")
	req, err := http.NewRequest(http.MethodPost, "/roles", role)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleCreate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, permTypes.ErrInvalidRoleName.Error()+"\n")
}

func (s *S) TestAddRoleNameAlreadyExists(c *check.C) {
	ctx := context.TODO()
	_, err := permission.NewRole(ctx, "ble", "global", "desc")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole(ctx, "ble")
	b := bytes.NewBufferString("name=ble&context=global")
	req, err := http.NewRequest(http.MethodPost, "/roles", b)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleCreate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, permTypes.ErrRoleAlreadyExists.Error()+"\n")
}

func (s *S) TestRemoveRole(c *check.C) {
	ctx := context.TODO()

	rolesCollection, err := storagev2.RolesCollection()
	c.Assert(err, check.IsNil)

	_, err = rolesCollection.DeleteMany(ctx, mongoBSON.M{})
	c.Assert(err, check.IsNil)

	_, err = permission.NewRole(ctx, "test", "app", "")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/roles/test", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleDelete,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	roles, err := permission.ListRoles(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(roles, check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "test"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveRoleWithUsers(c *check.C) {
	ctx := context.TODO()

	rolesCollection, err := storagev2.RolesCollection()
	c.Assert(err, check.IsNil)

	_, err = rolesCollection.DeleteMany(ctx, mongoBSON.M{})
	c.Assert(err, check.IsNil)

	_, err = permission.NewRole(ctx, "test", "app", "")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/roles/test", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleDelete,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	user, err := auth.ConvertNewUser(token.User(context.TODO()))
	c.Assert(err, check.IsNil)
	err = user.AddRole(ctx, "test", "app")
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusPreconditionFailed)
	c.Assert(recorder.Body.String(), check.Equals, permTypes.ErrRemoveRoleWithUsers.Error()+"\n")
	roles, err := permission.ListRoles(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(roles, check.HasLen, 2)
	user, err = auth.ConvertNewUser(token.User(context.TODO()))
	c.Assert(err, check.IsNil)
	c.Assert(user.Roles, check.HasLen, 2)
}

func (s *S) TestRemoveRoleUnauthorized(c *check.C) {
	ctx := context.TODO()
	_, err := permission.NewRole(ctx, "test", "app", "")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/roles/test", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestListRoles(c *check.C) {
	ctx := context.TODO()

	rolesCollection, err := storagev2.RolesCollection()
	c.Assert(err, check.IsNil)

	_, err = rolesCollection.DeleteMany(ctx, mongoBSON.M{})
	c.Assert(err, check.IsNil)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/roles", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	expected := `[{"name":"majortomrole.update","context":"global","Description":"","scheme_names":["role.update"]}]`
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Body.String(), check.Equals, expected)
}

func (s *S) TestRoleInfoNotFound(c *check.C) {
	ctx := context.TODO()

	rolesCollection, err := storagev2.RolesCollection()
	c.Assert(err, check.IsNil)

	_, err = rolesCollection.DeleteMany(ctx, mongoBSON.M{})
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/roles/xpto.update", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRoleInfo(c *check.C) {
	ctx := context.TODO()

	rolesCollection, err := storagev2.RolesCollection()
	c.Assert(err, check.IsNil)

	_, err = rolesCollection.DeleteMany(ctx, mongoBSON.M{})
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/roles/majortomrole.update", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	expected := `{"name":"majortomrole.update","context":"global","Description":"","scheme_names":["role.update"]}`
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Body.String(), check.Equals, expected)
}

func (s *S) TestAddPermissionsToARole(c *check.C) {
	ctx := context.TODO()
	_, err := permission.NewRole(ctx, "test", "team", "")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	b := bytes.NewBufferString(`permission=app.update&permission=app.deploy`)
	req, err := http.NewRequest(http.MethodPost, "/roles/test/permissions", b)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	r, err := permission.FindRole(ctx, "test")
	c.Assert(err, check.IsNil)
	sort.Strings(r.SchemeNames)
	c.Assert(r.SchemeNames, check.DeepEquals, []string{"app.deploy", "app.update"})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.permission.add",
		StartCustomData: []map[string]interface{}{
			{"name": "permission", "value": []string{"app.update", "app.deploy"}},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddPermissionsToARolePermissionNotFound(c *check.C) {
	ctx := context.TODO()
	_, err := permission.NewRole(ctx, "test", "team", "")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	b := bytes.NewBufferString(`permission=does.not.exists&permission=app.deploy`)
	req, err := http.NewRequest(http.MethodPost, "/roles/test/permissions", b)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(rec.Body.String(), check.Matches, "permission named .* not found\n")
}

func (s *S) TestAddPermissionsToARoleInvalidName(c *check.C) {
	ctx := context.TODO()
	_, err := permission.NewRole(ctx, "test", "team", "")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	b := bytes.NewBufferString(`permission=&permission=app.deploy`)
	req, err := http.NewRequest(http.MethodPost, "/roles/test/permissions", b)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(rec.Body.String(), check.Equals, permTypes.ErrInvalidPermissionName.Error()+"\n")
}

func (s *S) TestAddPermissionsToARolePermissionNotAllowed(c *check.C) {
	ctx := context.TODO()
	_, err := permission.NewRole(ctx, "test", "team", "")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	b := bytes.NewBufferString(`permission=pool.create`)
	req, err := http.NewRequest(http.MethodPost, "/roles/test/permissions", b)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusConflict)
	c.Assert(rec.Body.String(), check.Matches, "permission .* not allowed with context of type .*\n")
}

func (s *S) TestRemovePermissionsRoleNotFound(c *check.C) {
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodDelete, "/roles/test/permissions/app.update", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRemovePermissionsFromRole(c *check.C) {
	ctx := context.TODO()

	r, err := permission.NewRole(ctx, "test", "team", "")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole(ctx, r.Name)
	err = r.AddPermissions(context.TODO(), "app.update")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodDelete, "/roles/test/permissions/app.update", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	r, err = permission.FindRole(context.TODO(), "test")
	c.Assert(err, check.IsNil)
	c.Assert(r.SchemeNames, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.permission.remove",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "test"},
			{"name": ":permission", "value": "app.update"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAssignRole(c *check.C) {
	ctx := context.TODO()

	role, err := permission.NewRole(ctx, "test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(ctx, "app.create")
	c.Assert(err, check.IsNil)
	_, emptyToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=myteam", emptyToken.GetUserName()))
	req, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	emptyUser, err := emptyToken.User(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(emptyUser.Roles, check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.assign",
		StartCustomData: []map[string]interface{}{
			{"name": "email", "value": emptyToken.GetUserName()},
			{"name": "context", "value": "myteam"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAssignRoleNotFound(c *check.C) {
	_, emptyToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=myteam", emptyToken.GetUserName()))
	req, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRoleAssignEmptyContextValueAndGlobalContextType(c *check.C) {
	ctx := context.TODO()

	_, err := permission.NewRole(ctx, "test", "global", "")
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s", token.GetUserName()))
	req, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRoleAssignEmptyContextValueWithoutGlobalContextType(c *check.C) {
	ctx := context.TODO()
	_, err := permission.NewRole(ctx, "test", "team", "")
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s", token.GetUserName()))
	req, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxAppNotFound(c *check.C) {
	ctx := context.TODO()
	appName := "myapp"
	_, err := permission.NewRole(ctx, "test", "app", "")
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), appName))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxAppFound(c *check.C) {
	ctx := context.TODO()
	appName := "myapp"
	_, err := permission.NewRole(ctx, "test", "app", "")
	c.Assert(err, check.IsNil)
	user, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	app1 := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"a"}}
	err = app.CreateApp(context.TODO(), &app1, user)
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), appName))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRoleAssignValidateCtxTeamNotFound(c *check.C) {
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return nil, errors.New("not found")
	}
	team := "myteam"
	_, err := permission.NewRole(context.TODO(), "test", "team", "")
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), team))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxTeamFound(c *check.C) {
	team := "myteam"
	_, err := permission.NewRole(context.TODO(), "test", "team", "")
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), team))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRoleAssignValidateCtxUserNotFound(c *check.C) {
	user := "someuser@validemail.com"
	_, err := permission.NewRole(context.TODO(), "test", "user", "")
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), user))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxUserFound(c *check.C) {
	user, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "user", "")
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), user.Email))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRoleAssignValidateCtxPoolNotFound(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "pool", "")
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), "somepool"))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxPoolFound(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "pool", "")
	c.Assert(err, check.IsNil)
	poolName := "somepool"
	opts := pool.AddPoolOptions{Name: poolName, Public: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), poolName))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRoleAssignValidateCtxServiceNotFound(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "service", "")
	c.Assert(err, check.IsNil)
	serviceName := "someservice"
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), serviceName))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxServiceFound(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "service", "")
	c.Assert(err, check.IsNil)
	serviceName := "someservice"
	srv := service.Service{
		Name:       serviceName,
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = service.Create(context.TODO(), srv)
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), serviceName))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRoleAssignValidateCtxServiceInstanceNotFound(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "service-instance", "")
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), "my-instance"))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxServiceInstanceFound(c *check.C) {
	siName := "my-instance"
	si := service.ServiceInstance{
		Name:        siName,
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
		Description: "desc",
		TeamOwner:   s.team.Name,
	}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err = permission.NewRole(context.TODO(), "test", "service-instance", "")
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), siName))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRoleAssignValidateCtxVolumeNotFound(c *check.C) {
	s.mockService.VolumeService.OnGet = func(ctx context.Context, _ string) (*volume.Volume, error) {
		return nil, errors.New("not found")
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "volume", "")
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), "my-volume"))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxVolumeFound(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "volume", "")
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), "my-volume"))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRoleAssignValidateCtxRouterNotFound(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "router", "")
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), "my-router"))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleAssignValidateCtxRouterFound(c *check.C) {
	routerName := "dr1"
	dr := routerTypes.DynamicRouter{
		Name:   routerName,
		Type:   "fake",
		Config: map[string]interface{}{"a": "b"},
	}
	s.mockService.DynamicRouter.OnGet = func(name string) (*routerTypes.DynamicRouter, error) {
		return &dr, nil
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	_, err := permission.NewRole(context.TODO(), "test", "router", "")
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=%s", token.GetUserName(), routerName))
	req1, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req1)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestAssignRoleNotAuthorized(c *check.C) {
	role, err := permission.NewRole(context.TODO(), "test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(context.TODO(), "app.create")
	c.Assert(err, check.IsNil)
	_, emptyToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=myteam", emptyToken.GetUserName()))
	req, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "otherteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "User not authorized to use permission app.create(team myteam)\n")
	emptyUser, err := emptyToken.User(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(emptyUser.Roles, check.HasLen, 0)
}

func (s *S) TestDissociateRoleNotFound(c *check.C) {
	_, otherToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	url := fmt.Sprintf("/roles/test/user/%s?context=myteam", otherToken.GetUserName())
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestDissociateRole(c *check.C) {
	role, err := permission.NewRole(context.TODO(), "test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(context.TODO(), "app.create")
	c.Assert(err, check.IsNil)
	_, otherToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	otherUser, err := auth.ConvertNewUser(otherToken.User(context.TODO()))
	c.Assert(err, check.IsNil)
	err = otherUser.AddRole(context.TODO(), role.Name, "myteam")
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/roles/test/user/%s?context=myteam", otherToken.GetUserName())
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	otherUser, err = auth.ConvertNewUser(otherToken.User(context.TODO()))
	c.Assert(err, check.IsNil)
	c.Assert(otherUser.Roles, check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.dissociate",
		StartCustomData: []map[string]interface{}{
			{"name": ":email", "value": otherToken.GetUserName()},
			{"name": "context", "value": "myteam"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestDissociateRoleNotAuthorized(c *check.C) {
	role, err := permission.NewRole(context.TODO(), "test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(context.TODO(), "app.create")
	c.Assert(err, check.IsNil)
	_, otherToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	otherUser, err := auth.ConvertNewUser(otherToken.User(context.TODO()))
	c.Assert(err, check.IsNil)
	err = otherUser.AddRole(context.TODO(), role.Name, "myteam")
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/roles/test/user/%s?context=myteam", otherToken.GetUserName())
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "otherteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "User not authorized to use permission app.create(team myteam)\n")
	otherUser, err = auth.ConvertNewUser(otherToken.User(context.TODO()))
	c.Assert(err, check.IsNil)
	c.Assert(otherUser.Roles, check.HasLen, 1)
}

func (s *S) TestListPermissions(c *check.C) {
	role, err := permission.NewRole(context.TODO(), "test", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(context.TODO(), "app")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/permissions", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	var data []permissionSchemeData
	err = json.Unmarshal(rec.Body.Bytes(), &data)
	c.Assert(err, check.IsNil)
	c.Assert(len(data) > 0, check.Equals, true)
	c.Assert(data[0], check.DeepEquals, permissionSchemeData{
		Name:     "",
		Contexts: []string{"global"},
	})
}

func (s *S) TestAddDefaultRole(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "r1", "team", "")
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole(context.TODO(), "r2", "team", "")
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole(context.TODO(), "r3", "global", "")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString("team-create=r1&team-create=r2&user-create=r3")
	req, err := http.NewRequest(http.MethodPost, "/role/default", body)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleDefaultCreate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	r1, err := permission.FindRole(context.TODO(), "r1")
	c.Assert(err, check.IsNil)
	c.Assert(r1.Events, check.DeepEquals, []string{permTypes.RoleEventTeamCreate.String()})
	r2, err := permission.FindRole(context.TODO(), "r2")
	c.Assert(err, check.IsNil)
	c.Assert(r2.Events, check.DeepEquals, []string{permTypes.RoleEventTeamCreate.String()})
	r3, err := permission.FindRole(context.TODO(), "r3")
	c.Assert(err, check.IsNil)
	c.Assert(r3.Events, check.DeepEquals, []string{permTypes.RoleEventUserCreate.String()})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "r1"},
		Owner:  token.GetUserName(),
		Kind:   "role.default.create",
		StartCustomData: []map[string]interface{}{
			{"name": "team-create", "value": []string{"r1", "r2"}},
			{"name": "user-create", "value": "r3"},
		},
	}, eventtest.HasEvent)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "r2"},
		Owner:  token.GetUserName(),
		Kind:   "role.default.create",
		StartCustomData: []map[string]interface{}{
			{"name": "team-create", "value": []string{"r1", "r2"}},
			{"name": "user-create", "value": "r3"},
		},
	}, eventtest.HasEvent)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "r3"},
		Owner:  token.GetUserName(),
		Kind:   "role.default.create",
		StartCustomData: []map[string]interface{}{
			{"name": "team-create", "value": []string{"r1", "r2"}},
			{"name": "user-create", "value": "r3"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddDefaultRoleIncompatibleContext(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "r1", "team", "")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString("user-create=r1")
	req, err := http.NewRequest(http.MethodPost, "/role/default", body)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleDefaultCreate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(rec.Body.String(), check.Equals, "wrong context type for role event, expected \"global\" role has \"team\"\n")
}

func (s *S) TestAddDefaultRoleInvalidRole(c *check.C) {
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString("user-create=invalid")
	req, err := http.NewRequest(http.MethodPost, "/role/default", body)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleDefaultCreate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(rec.Body.String(), check.Equals, "role not found\n")
}

func (s *S) TestRemoveDefaultRole(c *check.C) {
	r1, err := permission.NewRole(context.TODO(), "r1", "team", "")
	c.Assert(err, check.IsNil)
	err = r1.AddEvent(context.TODO(), permTypes.RoleEventTeamCreate.String())
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodDelete, "/role/default?team-create=r1", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleDefaultDelete,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	r1, err = permission.FindRole(context.TODO(), "r1")
	c.Assert(err, check.IsNil)
	c.Assert(r1.Events, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "r1"},
		Owner:  token.GetUserName(),
		Kind:   "role.default.delete",
		StartCustomData: []map[string]interface{}{
			{"name": "team-create", "value": "r1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRoleUpdateDestroysAndCreatesNewRole(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	user, err := auth.ConvertNewUser(token.User(context.TODO()))
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole(context.TODO(), "r1", "app", "")
	c.Assert(err, check.IsNil)
	err = user.AddRole(context.TODO(), "r1", "app")
	c.Assert(err, check.IsNil)
	role := bytes.NewBufferString("name=r1&newName=r2&contextType=team&description=new+desc")
	req, err := http.NewRequest(http.MethodPut, "/roles", role)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "r1"},
		Owner:  token.GetUserName(),
		Kind:   "role.update",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "r1"},
			{"name": "newName", "value": "r2"},
			{"name": "contextType", "value": "team"},
			{"name": "description", "value": "new desc"},
		},
	}, eventtest.HasEvent)
	r, err := permission.FindRole(context.TODO(), "r2")
	c.Assert(err, check.IsNil)
	c.Assert(string(r.ContextType), check.Equals, "team")
	c.Assert(string(r.Description), check.Equals, "new desc")
	users, err := auth.ListUsersWithRole(context.TODO(), "r2")
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
}

func (s *S) TestRoleUpdateUnauthorized(c *check.C) {
	token := userWithPermission(c)
	role := bytes.NewBufferString("name=r1&newName=&contextType=app&description=")
	req, err := http.NewRequest(http.MethodPut, "/roles", role)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRoleUpdateWithoutFields(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	role := bytes.NewBufferString("name=r1&newName=&contextType=&description=")
	req, err := http.NewRequest(http.MethodPut, "/roles", role)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleUpdateIncorrectContext(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	user, err := auth.ConvertNewUser(token.User(context.TODO()))
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole(context.TODO(), "r1", "app", "")
	c.Assert(err, check.IsNil)
	err = user.AddRole(context.TODO(), "r1", "app")
	c.Assert(err, check.IsNil)
	role := bytes.NewBufferString("name=r1&newName=&contextType=yasuo&description=")
	req, err := http.NewRequest(http.MethodPut, "/roles", role)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Body.String(), check.Equals, "invalid context type \"yasuo\"\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRoleUpdateSingleField(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	user, err := auth.ConvertNewUser(token.User(context.TODO()))
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole(context.TODO(), "r1", "app", "Syncopy")
	c.Assert(err, check.IsNil)
	err = user.AddRole(context.TODO(), "r1", "app")
	c.Assert(err, check.IsNil)
	role := bytes.NewBufferString("name=r1&newName=&contextType=team&description=")
	req, err := http.NewRequest(http.MethodPut, "/roles", role)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "r1"},
		Owner:  token.GetUserName(),
		Kind:   "role.update",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "r1"},
			{"name": "newName", "value": ""},
			{"name": "contextType", "value": "team"},
			{"name": "description", "value": ""},
		},
	}, eventtest.HasEvent)
	r, err := permission.FindRole(context.TODO(), "r1")
	c.Assert(err, check.IsNil)
	c.Assert(string(r.ContextType), check.Equals, "team")
	c.Assert(string(r.Description), check.Equals, "Syncopy")
	users, err := auth.ListUsersWithRole(context.TODO(), "r1")
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
}

func (s *S) TestAssignRoleToTeamToken(c *check.C) {
	app1 := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"a"}}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	teamToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team: s.team.Name,
	}, s.token)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`context=myapp&token_id=` + teamToken.TokenID)
	req, err := http.NewRequest(http.MethodPost, "/1.6/roles/newrole/token", body)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), teamToken.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.DeepEquals, []authTypes.RoleInstance{
		{Name: "newrole", ContextValue: "myapp"},
	})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "newrole"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.assign",
		StartCustomData: []map[string]interface{}{
			{"name": "token_id", "value": teamToken.TokenID},
			{"name": "context", "value": "myapp"},
			{"name": ":name", "value": "newrole"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAssignRoleToTeamTokenRoleNotFound(c *check.C) {
	teamToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team: s.team.Name,
	}, s.token)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`context=myapp&token_id=` + teamToken.TokenID)
	req, err := http.NewRequest(http.MethodPost, "/1.6/roles/rolenotfound/token", body)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "rolenotfound"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.assign",
		StartCustomData: []map[string]interface{}{
			{"name": "token_id", "value": teamToken.TokenID},
			{"name": "context", "value": "myapp"},
			{"name": ":name", "value": "rolenotfound"},
		},
		ErrorMatches: "role not found",
	}, eventtest.HasEvent)
}

func (s *S) TestAssignTokenRoleToEmptyTeam(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "team", "")
	c.Assert(err, check.IsNil)
	teamToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team: s.team.Name,
	}, s.token)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`token_id=` + teamToken.TokenID)
	req, err := http.NewRequest(http.MethodPost, "/1.6/roles/newrole/token", body)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestAssignRoleToTeamTokenNotAuthorized(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	teamToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team: s.team.Name,
	}, s.token)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`context=myapp&token_id=` + teamToken.TokenID)
	req, err := http.NewRequest(http.MethodPost, "/1.6/roles/newrole/token", body)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "otherteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestDissociateRoleFromTeamToken(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	teamToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team: s.team.Name,
	}, s.token)
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), teamToken.TokenID, "newrole", "myapp")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("/1.6/roles/newrole/token/%s?context=myapp", teamToken.TokenID),
		nil,
	)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), teamToken.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "newrole"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.dissociate",
		StartCustomData: []map[string]interface{}{
			{"name": ":token_id", "value": teamToken.TokenID},
			{"name": "context", "value": "myapp"},
			{"name": ":name", "value": "newrole"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestDissociateRoleFromTeamTokenRoleNotFound(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	teamToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team: s.team.Name,
	}, s.token)
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), teamToken.TokenID, "newrole", "myapp")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("/1.6/roles/rolenotfound/token/%s?context=myapp", teamToken.TokenID),
		nil,
	)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), teamToken.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "rolenotfound"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.dissociate",
		StartCustomData: []map[string]interface{}{
			{"name": ":token_id", "value": teamToken.TokenID},
			{"name": "context", "value": "myapp"},
			{"name": ":name", "value": "rolenotfound"},
		},
		ErrorMatches: "role not found",
	}, eventtest.HasEvent)
}

func (s *S) TestDissociateRoleFromTeamTokenNotAuthorized(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	teamToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team: s.team.Name,
	}, s.token)
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), teamToken.TokenID, "newrole", "myapp")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("/1.6/roles/rolenotfound/token/%s?context=myapp", teamToken.TokenID),
		nil,
	)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), teamToken.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.HasLen, 1)
}

func (s *S) TestAssignRoleToAuthGroup(c *check.C) {
	app1 := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"a"}}
	err := app.CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`context=myapp&group_name=g1&contextType=app`)
	req, err := http.NewRequest(http.MethodPost, "/1.9/roles/newrole/group", body)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %s", recorder.Body.String()))
	groups, err := servicemanager.AuthGroup.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.DeepEquals, []authTypes.Group{
		{
			Name: "g1",
			Roles: []authTypes.RoleInstance{
				{
					Name:         "newrole",
					ContextValue: "myapp",
				},
			},
		},
	})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "newrole"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.assign",
		StartCustomData: []map[string]interface{}{
			{"name": "group_name", "value": "g1"},
			{"name": "context", "value": "myapp"},
			{"name": ":name", "value": "newrole"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAssignRoleToAuthGroupRoleNotFound(c *check.C) {
	body := strings.NewReader(`context=myapp&group_name=g1`)
	req, err := http.NewRequest(http.MethodPost, "/1.9/roles/rolenotfound/group", body)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "rolenotfound"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.assign",
		StartCustomData: []map[string]interface{}{
			{"name": "group_name", "value": "g1"},
			{"name": "context", "value": "myapp"},
			{"name": ":name", "value": "rolenotfound"},
		},
		ErrorMatches: "role not found",
	}, eventtest.HasEvent)
}

func (s *S) TestAssignRoleToAuthGroupNotAuthorized(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`context=myapp&group_name=g1`)
	req, err := http.NewRequest(http.MethodPost, "/1.9/roles/newrole/group", body)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "otherteam"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestDissociateRoleFromAuthGroup(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole(context.TODO(), "g1", "newrole", "myapp")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete,
		"/1.9/roles/newrole/group/g1?context=myapp",
		nil,
	)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	groups, err := servicemanager.AuthGroup.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.DeepEquals, []authTypes.Group{
		{
			Name:  "g1",
			Roles: []authTypes.RoleInstance{},
		},
	})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "newrole"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.dissociate",
		StartCustomData: []map[string]interface{}{
			{"name": ":group_name", "value": "g1"},
			{"name": "context", "value": "myapp"},
			{"name": ":name", "value": "newrole"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestDissociateRoleFromAuthGroupRoleNotFound(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole(context.TODO(), "g1", "newrole", "myapp")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete,
		"/1.9/roles/rolenotfound/group/g1?context=myapp",
		nil,
	)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	groups, err := servicemanager.AuthGroup.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.DeepEquals, []authTypes.Group{
		{
			Name: "g1",
			Roles: []authTypes.RoleInstance{
				{
					Name:         "newrole",
					ContextValue: "myapp",
				},
			},
		},
	})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeRole, Value: "rolenotfound"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.dissociate",
		StartCustomData: []map[string]interface{}{
			{"name": ":group_name", "value": "g1"},
			{"name": "context", "value": "myapp"},
			{"name": ":name", "value": "rolenotfound"},
		},
		ErrorMatches: "role not found",
	}, eventtest.HasEvent)
}

func (s *S) TestDissociateRoleFromAuthGroupNotAuthorized(c *check.C) {
	_, err := permission.NewRole(context.TODO(), "newrole", "app", "")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole(context.TODO(), "g1", "newrole", "myapp")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete,
		"/1.9/roles/newrole/group/g1?context=myapp",
		nil,
	)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "myteam"),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
	groups, err := servicemanager.AuthGroup.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.DeepEquals, []authTypes.Group{
		{
			Name: "g1",
			Roles: []authTypes.RoleInstance{
				{
					Name:         "newrole",
					ContextValue: "myapp",
				},
			},
		},
	})
}
