package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
)

func (s *S) TestAddRole(c *check.C) {
	s.conn.Roles().DropCollection()
	role := bytes.NewBufferString("name=test&context=global")
	req, err := http.NewRequest("POST", "/roles", role)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleCreate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	roles, err := permission.ListRoles()
	c.Assert(err, check.IsNil)
	c.Assert(roles, check.HasLen, 2)
}

func (s *S) TestAddRoleUnauthorized(c *check.C) {
	role := bytes.NewBufferString("name=test&context=global")
	req, err := http.NewRequest("POST", "/roles", role)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRemoveRole(c *check.C) {
	s.conn.Roles().DropCollection()
	_, err := permission.NewRole("test", "app")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("DELETE", "/roles/test", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleDelete,
		Context: permission.Context(permission.CtxGlobal, ""),
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
}

func (s *S) TestRemoveRoleUnauthorized(c *check.C) {
	_, err := permission.NewRole("test", "app")
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("DELETE", "/roles/test", nil)
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
	s.conn.Roles().DropCollection()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/roles", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	expected := `[{"name":"majortomrole.update","context":"global","scheme_names":["role.update"]}]`
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Body.String(), check.Equals, expected)
}

func (s *S) TestAddPermissionsToARole(c *check.C) {
	_, err := permission.NewRole("test", "team")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	b := bytes.NewBufferString(`permission=app.update&permission=app.deploy`)
	req, err := http.NewRequest("POST", "/roles/test/permissions", b)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permission.CtxGlobal, ""),
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
}

func (s *S) TestAddPermissionsToARoleSyncGitRepository(c *check.C) {
	_, err := permission.NewRole("test", "team")
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "userWithRole@groundcontrol.com", Password: "123456"}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	err = user.AddRole("test", s.team.Name)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	users, err := repositorytest.Granted("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(users, check.DeepEquals, []string{s.user.Email})
	rec := httptest.NewRecorder()
	b := bytes.NewBufferString(`permission=app.update&permission=app.deploy`)
	req, err := http.NewRequest("POST", "/roles/test/permissions", b)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permission.CtxGlobal, ""),
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

func (s *S) TestRemovePermissionsFromRole(c *check.C) {
	r, err := permission.NewRole("test", "team")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole(r.Name)
	err = r.AddPermissions("app.update")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "/roles/test/permissions/app.update", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	r, err = permission.FindRole("test")
	c.Assert(err, check.IsNil)
	c.Assert(r.SchemeNames, check.DeepEquals, []string{})
}

func (s *S) TestRemovePermissionsFromRoleSyncGitRepository(c *check.C) {
	r, err := permission.NewRole("test", "team")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole(r.Name)
	err = r.AddPermissions("app.deploy")
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "userWithRole@groundcontrol.com", Password: "123456"}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	err = user.AddRole("test", s.team.Name)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	err = repository.Manager().GrantAccess(a.Name, user.Email)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "/roles/test/permissions/app.deploy", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permission.CtxGlobal, ""),
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
	_, err := permission.NewRole("test", "team")
	c.Assert(err, check.IsNil)
	role := bytes.NewBufferString("email=majortom@groundcontrol.com&context=myteam")
	req, err := http.NewRequest("POST", "/roles/test/user", role)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdateAssign,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	u, err := auth.GetUserByEmail("majortom@groundcontrol.com")
	c.Assert(err, check.IsNil)
	c.Assert(u.Roles, check.HasLen, 2)
}

func (s *S) TestDissociateRole(c *check.C) {
	_, err := permission.NewRole("test", "team")
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdateDissociate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	u, err := auth.GetUserByEmail("majortom@groundcontrol.com")
	c.Assert(err, check.IsNil)
	err = u.AddRole("test", "myteam")
	c.Assert(err, check.IsNil)
	c.Assert(u.Roles, check.HasLen, 2)
	req, err := http.NewRequest("DELETE", "/roles/test/user/majortom@groundcontrol.com?context=myteam", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, req)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	u, err = auth.GetUserByEmail("majortom@groundcontrol.com")
	c.Assert(err, check.IsNil)
	c.Assert(u.Roles, check.HasLen, 1)
}

func (s *S) TestListPermissions(c *check.C) {
	role, err := permission.NewRole("test", "app")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/permissions", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permission.CtxGlobal, ""),
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
