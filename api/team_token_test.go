// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func (s *S) TestTeamTokenList(c *check.C) {
	teamToken1 := authTypes.TeamToken{Token: "12345", Teams: []string{s.team.Name}}
	err := auth.TeamTokenService().Insert(teamToken1)
	c.Assert(err, check.IsNil)
	teamToken2 := authTypes.TeamToken{Token: "abc", Teams: []string{"otherteam"}}
	err = auth.TeamTokenService().Insert(teamToken2)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/teamtokens", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []authTypes.TeamToken
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []authTypes.TeamToken{teamToken1})
}

func (s *S) TestTeamTokenListEmpty(c *check.C) {
	teamToken := authTypes.TeamToken{Token: "abc", Teams: []string{"otherteam"}}
	err := auth.TeamTokenService().Insert(teamToken)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/teamtokens", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestTeamTokenListNoPermission(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/teamtokens", nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permission.CtxApp, app1.Name),
	})
	request.Header.Set("authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestTeamTokenCreate(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/1.6/apps/%s/tokens", app1.Name)
	request, err := http.NewRequest("POST", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var result authTypes.TeamToken
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Token, check.NotNil)
	c.Assert(result.AppName, check.Equals, app1.Name)
	c.Assert(result.CreatorEmail, check.Equals, s.token.GetUserName())

	results, err := auth.TeamTokenService().FindByAppName(app1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(results, check.HasLen, 1)
	c.Assert(results[0].AppName, check.Equals, app1.Name)
	c.Assert(results[0].Token, check.Not(check.HasLen), 0)

	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeApp, Value: app1.Name},
		Owner:  s.user.Email,
		Kind:   "app.token.create",
	}, eventtest.HasEvent)
}

func (s *S) TestTeamTokenCreateNoPermission(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/1.6/apps/%s/tokens", app1.Name)
	request, err := http.NewRequest("POST", url, nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppTokenRead,
		Context: permission.Context(permission.CtxApp, app1.Name),
	})
	request.Header.Set("authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestTeamTokenDelete(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	appToken := authTypes.TeamToken{Token: "12345", AppName: app1.Name, CreatorEmail: s.team.Name}
	err = auth.TeamTokenService().Insert(appToken)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/1.6/apps/%s/tokens/%s", app1.Name, appToken.Token)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	results, err := auth.TeamTokenService().FindByAppName(app1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(results, check.HasLen, 0)

	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeApp, Value: app1.Name},
		Owner:  s.user.Email,
		Kind:   "app.token.delete",
	}, eventtest.HasEvent)
}

func (s *S) TestTeamTokenDeleteTokenNotFound(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/1.6/apps/%s/tokens/abc123", app1.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestTeamTokenDeleteNoPermission(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	appToken := authTypes.TeamToken{Token: "12345", AppName: app1.Name, CreatorEmail: s.team.Name}
	err = auth.TeamTokenService().Insert(appToken)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/1.6/apps/%s/tokens/%s", app1.Name, appToken.Token)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppTokenCreate,
		Context: permission.Context(permission.CtxApp, app1.Name),
	})
	request.Header.Set("authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
