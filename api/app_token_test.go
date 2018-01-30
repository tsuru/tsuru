// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func (s *S) TestAppTokenList(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	appToken := authTypes.AppToken{Token: "12345", AppName: app1.Name, CreatorEmail: s.team.Name}
	err = auth.AppTokenService().Insert(appToken)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/1.6/apps/%s/tokens", app1.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []authTypes.AppToken
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []authTypes.AppToken{appToken})
}

func (s *S) TestAppTokenListEmpty(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/1.6/apps/%s/tokens", app1.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestAppTokenListNoPermission(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/1.6/apps/%s/tokens", app1.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permission.CtxPool, "otherpool"),
	})
	request.Header.Set("authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
