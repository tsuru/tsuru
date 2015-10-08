package api

import (
	"bytes"
	"github.com/tsuru/tsuru/permission"
	"gopkg.in/check.v1"
	"net/http"
	"net/http/httptest"
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
