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

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestAddRole(c *check.C) {
	s.conn.Roles().RemoveAll(nil)
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
	roles, err := permission.ListRoles()
	c.Assert(err, check.IsNil)
	c.Assert(roles, check.HasLen, 2)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "test"},
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
	_, err := permission.NewRole("ble", "global", "desc")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole("ble")
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
	s.conn.Roles().RemoveAll(nil)
	_, err := permission.NewRole("test", "app", "")
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
	roles, err := permission.ListRoles()
	c.Assert(err, check.IsNil)
	c.Assert(roles, check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "test"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveRoleWithUsers(c *check.C) {
	s.conn.Roles().RemoveAll(nil)
	_, err := permission.NewRole("test", "app", "")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/roles/test", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleDelete,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	user, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	err = user.AddRole("test", "app")
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusPreconditionFailed)
	c.Assert(recorder.Body.String(), check.Equals, permTypes.ErrRemoveRoleWithUsers.Error()+"\n")
	roles, err := permission.ListRoles()
	c.Assert(err, check.IsNil)
	c.Assert(roles, check.HasLen, 2)
	user, err = auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	c.Assert(user.Roles, check.HasLen, 2)
}

func (s *S) TestRemoveRoleUnauthorized(c *check.C) {
	_, err := permission.NewRole("test", "app", "")
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
	s.conn.Roles().RemoveAll(nil)
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
	s.conn.Roles().RemoveAll(nil)
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
	s.conn.Roles().RemoveAll(nil)
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
	_, err := permission.NewRole("test", "team", "")
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
	r, err := permission.FindRole("test")
	c.Assert(err, check.IsNil)
	sort.Strings(r.SchemeNames)
	c.Assert(r.SchemeNames, check.DeepEquals, []string{"app.deploy", "app.update"})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.permission.add",
		StartCustomData: []map[string]interface{}{
			{"name": "permission", "value": []string{"app.update", "app.deploy"}},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddPermissionsToARolePermissionNotFound(c *check.C) {
	_, err := permission.NewRole("test", "team", "")
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
	_, err := permission.NewRole("test", "team", "")
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
	_, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	b := bytes.NewBufferString(`permission=node.create`)
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

func (s *S) TestAddPermissionsToARoleSyncGitRepository(c *check.C) {
	_, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "userWithRole@groundcontrol.com", Password: "123456"}
	_, err = nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	err = user.AddRole("test", s.team.Name)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	users, err := repositorytest.Granted("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(users, check.DeepEquals, []string{s.user.Email})
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
	users, err = repositorytest.Granted("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(users, check.DeepEquals, []string{s.user.Email, user.Email})
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
	r, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole(r.Name)
	err = r.AddPermissions("app.update")
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
	r, err = permission.FindRole("test")
	c.Assert(err, check.IsNil)
	c.Assert(r.SchemeNames, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.permission.remove",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "test"},
			{"name": ":permission", "value": "app.update"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemovePermissionsFromRoleSyncGitRepository(c *check.C) {
	r, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole(r.Name)
	err = r.AddPermissions("app.deploy")
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "userWithRole@groundcontrol.com", Password: "123456"}
	_, err = nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	err = user.AddRole("test", s.team.Name)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = repository.Manager().GrantAccess(context.TODO(), a.Name, user.Email)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodDelete, "/roles/test/permissions/app.deploy", nil)
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
	r, err = permission.FindRole("test")
	c.Assert(err, check.IsNil)
	c.Assert(r.SchemeNames, check.DeepEquals, []string{})
	users, err := repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.DeepEquals, []string{s.user.Email})
}

func (s *S) TestAssignRole(c *check.C) {
	role, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.create")
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
	emptyUser, err := emptyToken.User()
	c.Assert(err, check.IsNil)
	c.Assert(emptyUser.Roles, check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "test"},
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

func (s *S) TestAssignRoleNotAuthorized(c *check.C) {
	role, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.create")
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
	emptyUser, err := emptyToken.User()
	c.Assert(err, check.IsNil)
	c.Assert(emptyUser.Roles, check.HasLen, 0)
}

func (s *S) TestAssignRoleCheckGandalf(c *check.C) {
	role, err := permission.NewRole("test", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.deploy")
	c.Assert(err, check.IsNil)
	_, emptyToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	roleBody := bytes.NewBufferString(fmt.Sprintf("email=%s&context=myapp", emptyToken.GetUserName()))
	req, err := http.NewRequest(http.MethodPost, "/roles/test/user", roleBody)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxApp, "myapp"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	emptyUser, err := emptyToken.User()
	c.Assert(err, check.IsNil)
	users, err := repositorytest.Granted("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(users, check.DeepEquals, []string{s.user.Email, emptyToken.GetUserName()})
	c.Assert(emptyUser.Roles, check.HasLen, 1)
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
	role, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.create")
	c.Assert(err, check.IsNil)
	_, otherToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	otherUser, err := auth.ConvertNewUser(otherToken.User())
	c.Assert(err, check.IsNil)
	err = otherUser.AddRole(role.Name, "myteam")
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
	otherUser, err = auth.ConvertNewUser(otherToken.User())
	c.Assert(err, check.IsNil)
	c.Assert(otherUser.Roles, check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "test"},
		Owner:  token.GetUserName(),
		Kind:   "role.update.dissociate",
		StartCustomData: []map[string]interface{}{
			{"name": ":email", "value": otherToken.GetUserName()},
			{"name": "context", "value": "myteam"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestDissociateRoleNotAuthorized(c *check.C) {
	role, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.create")
	c.Assert(err, check.IsNil)
	_, otherToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	otherUser, err := auth.ConvertNewUser(otherToken.User())
	c.Assert(err, check.IsNil)
	err = otherUser.AddRole(role.Name, "myteam")
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
	otherUser, err = auth.ConvertNewUser(otherToken.User())
	c.Assert(err, check.IsNil)
	c.Assert(otherUser.Roles, check.HasLen, 1)
}

func (s *S) TestDissociateRoleCheckGandalf(c *check.C) {
	role, err := permission.NewRole("test", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.deploy")
	c.Assert(err, check.IsNil)
	_, otherToken := permissiontest.CustomUserWithPermission(c, nativeScheme, "user2")
	otherUser, err := auth.ConvertNewUser(otherToken.User())
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = otherUser.AddRole(role.Name, "myapp")
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/roles/test/user/%s?context=myapp", otherToken.GetUserName())
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	c.Assert(err, check.IsNil)
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "user1", permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxApp, "myapp"),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	otherUser, err = auth.ConvertNewUser(otherToken.User())
	c.Assert(err, check.IsNil)
	c.Assert(otherUser.Roles, check.HasLen, 0)
	users, err := repositorytest.Granted("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(users, check.DeepEquals, []string{s.user.Email})
}

func (s *S) TestListPermissions(c *check.C) {
	role, err := permission.NewRole("test", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app")
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
	_, err := permission.NewRole("r1", "team", "")
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("r2", "team", "")
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("r3", "global", "")
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
	r1, err := permission.FindRole("r1")
	c.Assert(err, check.IsNil)
	c.Assert(r1.Events, check.DeepEquals, []string{permTypes.RoleEventTeamCreate.String()})
	r2, err := permission.FindRole("r2")
	c.Assert(err, check.IsNil)
	c.Assert(r2.Events, check.DeepEquals, []string{permTypes.RoleEventTeamCreate.String()})
	r3, err := permission.FindRole("r3")
	c.Assert(err, check.IsNil)
	c.Assert(r3.Events, check.DeepEquals, []string{permTypes.RoleEventUserCreate.String()})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "r1"},
		Owner:  token.GetUserName(),
		Kind:   "role.default.create",
		StartCustomData: []map[string]interface{}{
			{"name": "team-create", "value": []string{"r1", "r2"}},
			{"name": "user-create", "value": "r3"},
		},
	}, eventtest.HasEvent)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "r2"},
		Owner:  token.GetUserName(),
		Kind:   "role.default.create",
		StartCustomData: []map[string]interface{}{
			{"name": "team-create", "value": []string{"r1", "r2"}},
			{"name": "user-create", "value": "r3"},
		},
	}, eventtest.HasEvent)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "r3"},
		Owner:  token.GetUserName(),
		Kind:   "role.default.create",
		StartCustomData: []map[string]interface{}{
			{"name": "team-create", "value": []string{"r1", "r2"}},
			{"name": "user-create", "value": "r3"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddDefaultRoleIncompatibleContext(c *check.C) {
	_, err := permission.NewRole("r1", "team", "")
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
	r1, err := permission.NewRole("r1", "team", "")
	c.Assert(err, check.IsNil)
	err = r1.AddEvent(permTypes.RoleEventTeamCreate.String())
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
	r1, err = permission.FindRole("r1")
	c.Assert(err, check.IsNil)
	c.Assert(r1.Events, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "r1"},
		Owner:  token.GetUserName(),
		Kind:   "role.default.delete",
		StartCustomData: []map[string]interface{}{
			{"name": "team-create", "value": "r1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) benchmarkAddPermissionToRole(c *check.C, body string) []string {
	c.StopTimer()
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.create")
	c.Assert(err, check.IsNil)
	nUsers := 100
	var userEmails []string
	for i := 0; i < nUsers; i++ {
		email := fmt.Sprintf("user-%d@somewhere.com", i)
		userEmails = append(userEmails, email)
		user := &auth.User{Email: email, Password: "123456"}
		_, err = nativeScheme.Create(context.TODO(), user)
		c.Assert(err, check.IsNil)
		err = user.AddRole("test", s.team.Name)
		c.Assert(err, check.IsNil)
	}
	recorder := httptest.NewRecorder()
	c.StartTimer()
	for i := 0; i < c.N; i++ {
		b := bytes.NewBufferString(body)
		request, err := http.NewRequest(http.MethodPost, "/roles/test/permissions", b)
		c.Assert(err, check.IsNil)
		request.Header.Add("Authorization", "bearer "+s.token.GetValue())
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		s.testServer.ServeHTTP(recorder, request)
	}
	c.StopTimer()
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	return userEmails
}

func (s *S) BenchmarkAddPermissionToRoleWithDeploy(c *check.C) {
	userEmails := s.benchmarkAddPermissionToRole(c, `permission=app.update&permission=app.deploy`)
	users, err := repositorytest.Granted("myapp")
	c.Assert(err, check.IsNil)
	userEmails = append(userEmails, s.user.Email)
	sort.Strings(users)
	sort.Strings(userEmails)
	c.Assert(users, check.DeepEquals, userEmails)
}

func (s *S) BenchmarkAddPermissionToRoleWithoutDeploy(c *check.C) {
	s.benchmarkAddPermissionToRole(c, `permission=app.update&permission=app.read`)
	users, err := repositorytest.Granted("myapp")
	c.Assert(err, check.IsNil)
	sort.Strings(users)
	c.Assert(users, check.DeepEquals, []string{s.user.Email})
}

func (s *S) TestRoleUpdateDestroysAndCreatesNewRole(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	user, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	err = user.AddRole("r1", "app")
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "r1"},
		Owner:  token.GetUserName(),
		Kind:   "role.update",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "r1"},
			{"name": "newName", "value": "r2"},
			{"name": "contextType", "value": "team"},
			{"name": "description", "value": "new desc"},
		},
	}, eventtest.HasEvent)
	r, err := permission.FindRole("r2")
	c.Assert(err, check.IsNil)
	c.Assert(string(r.ContextType), check.Equals, "team")
	c.Assert(string(r.Description), check.Equals, "new desc")
	users, err := auth.ListUsersWithRole("r2")
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
	user, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("r1", "app", "")
	c.Assert(err, check.IsNil)
	err = user.AddRole("r1", "app")
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
	user, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("r1", "app", "Syncopy")
	c.Assert(err, check.IsNil)
	err = user.AddRole("r1", "app")
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "r1"},
		Owner:  token.GetUserName(),
		Kind:   "role.update",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "r1"},
			{"name": "newName", "value": ""},
			{"name": "contextType", "value": "team"},
			{"name": "description", "value": ""},
		},
	}, eventtest.HasEvent)
	r, err := permission.FindRole("r1")
	c.Assert(err, check.IsNil)
	c.Assert(string(r.ContextType), check.Equals, "team")
	c.Assert(string(r.Description), check.Equals, "Syncopy")
	users, err := auth.ListUsersWithRole("r1")
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
}

func (s *S) TestAssignRoleToTeamToken(c *check.C) {
	_, err := permission.NewRole("newrole", "app", "")
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "newrole"},
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "rolenotfound"},
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

func (s *S) TestAssignRoleToTeamTokenNotAuthorized(c *check.C) {
	_, err := permission.NewRole("newrole", "app", "")
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
	_, err := permission.NewRole("newrole", "app", "")
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "newrole"},
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
	_, err := permission.NewRole("newrole", "app", "")
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "rolenotfound"},
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
	_, err := permission.NewRole("newrole", "app", "")
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
	_, err := permission.NewRole("newrole", "app", "")
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`context=myapp&group_name=g1`)
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
	groups, err := servicemanager.AuthGroup.List(nil)
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "newrole"},
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "rolenotfound"},
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
	_, err := permission.NewRole("newrole", "app", "")
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
	_, err := permission.NewRole("newrole", "app", "")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole("g1", "newrole", "myapp")
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
	groups, err := servicemanager.AuthGroup.List(nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.DeepEquals, []authTypes.Group{
		{
			Name:  "g1",
			Roles: []authTypes.RoleInstance{},
		},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeRole, Value: "newrole"},
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
	_, err := permission.NewRole("newrole", "app", "")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole("g1", "newrole", "myapp")
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
	groups, err := servicemanager.AuthGroup.List(nil)
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
		Target: event.Target{Type: event.TargetTypeRole, Value: "rolenotfound"},
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
	_, err := permission.NewRole("newrole", "app", "")
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole("g1", "newrole", "myapp")
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
	groups, err := servicemanager.AuthGroup.List(nil)
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
