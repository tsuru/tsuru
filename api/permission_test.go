package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/permission"
	"gopkg.in/check.v1"
)

func (s *S) TestAddRole(c *check.C) {
	rec := httptest.NewRecorder()
	role := bytes.NewBufferString(`{"name": "test", "context": "global"}`)
	req, err := http.NewRequest("POST", "/role", role)
	c.Assert(err, check.IsNil)
	err = addRole(rec, req, nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveRole(c *check.C) {
	_, err := permission.NewRole("test", "app")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	role := bytes.NewBufferString(`{"name": "test"}`)
	req, err := http.NewRequest("DELETE", "/role", role)
	c.Assert(err, check.IsNil)
	err = removeRole(rec, req, nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestListRoles(c *check.C) {
	_, err := permission.NewRole("test", "app")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/role", nil)
	c.Assert(err, check.IsNil)
	expected := `[{"name":"test","context":"app"}]`
	err = listRoles(rec, req, nil)
	c.Assert(err, check.IsNil)
	c.Assert(rec.Body.String(), check.Equals, expected)
}

func (s *S) TestAddPermissionsToARole(c *check.C) {
	r, err := permission.NewRole("test", "team")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole(r.Name)
	rec := httptest.NewRecorder()
	url := fmt.Sprintf("/role/%s/permissions?:name=%s", r.Name, r.Name)
	b := bytes.NewBufferString(`{"permissions": ["app.update"]}`)
	req, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	err = addPermissions(rec, req, nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemovePermissionsFromRole(c *check.C) {
	r, err := permission.NewRole("test", "team")
	c.Assert(err, check.IsNil)
	defer permission.DestroyRole(r.Name)
	err = r.AddPermissions("app.update")
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	url := fmt.Sprintf("/role/%s/permissions?:name=%s", r.Name, r.Name)
	b := bytes.NewBufferString(`{"permissions": ["app.update"]}`)
	req, err := http.NewRequest("DELETE", url, b)
	c.Assert(err, check.IsNil)
	err = removePermissions(rec, req, nil)
	c.Assert(err, check.IsNil)
}
