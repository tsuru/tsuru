// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func normalizeTimestamps(c *check.C, v []authTypes.TeamToken) []authTypes.TeamToken {
	sort.Slice(v, func(i, j int) bool {
		return v[i].TokenID < v[j].TokenID
	})
	for i, t := range v {
		c.Assert(t.CreatedAt.IsZero(), check.Equals, false)
		v[i].CreatedAt = time.Time{}
	}
	return v
}

func (s *S) TestTeamTokenList(c *check.C) {
	teamToken1, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "id1",
	}, s.token)
	c.Assert(err, check.IsNil)
	teamToken2, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "id2",
	}, s.token)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/tokens", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []authTypes.TeamToken
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(normalizeTimestamps(c, result), check.DeepEquals,
		normalizeTimestamps(c, []authTypes.TeamToken{
			teamToken1,
			teamToken2,
		}),
	)
}

func (s *S) TestTeamTokenListEmpty(c *check.C) {
	request, err := http.NewRequest("GET", "/1.6/tokens", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestTeamTokenCreate(c *check.C) {
	body := strings.NewReader(`token_id=t1&description=desc&expires_in=60&team=` + s.team.Name)
	request, err := http.NewRequest("POST", "/1.6/tokens", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated, check.Commentf("body: %q", recorder.Body.String()))
	var result authTypes.TeamToken
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Token, check.Not(check.Equals), "")
	c.Assert(result.CreatedAt.IsZero(), check.Equals, false)
	c.Assert(result.ExpiresAt.IsZero(), check.Equals, false)
	result.CreatedAt = time.Time{}
	result.ExpiresAt = time.Time{}
	result.Token = ""
	c.Assert(result, check.DeepEquals, authTypes.TeamToken{
		Team:         s.team.Name,
		TokenID:      "t1",
		CreatorEmail: s.token.GetUserName(),
		Description:  "desc",
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeTeam, Value: s.team.Name},
		Owner:  s.user.Email,
		Kind:   "team.token.create",
		StartCustomData: []map[string]interface{}{
			{"name": "token_id", "value": "t1"},
			{"name": "description", "value": "desc"},
			{"name": "expires_in", "value": "60"},
			{"name": "team", "value": s.team.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestTeamTokenCreateAutomaticTeam(c *check.C) {
	body := strings.NewReader(`token_id=t1&description=desc&expires_in=60`)
	request, err := http.NewRequest("POST", "/1.6/tokens", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated, check.Commentf("body: %q", recorder.Body.String()))
	var result authTypes.TeamToken
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Token, check.Not(check.Equals), "")
	c.Assert(result.CreatedAt.IsZero(), check.Equals, false)
	c.Assert(result.ExpiresAt.IsZero(), check.Equals, false)
	result.CreatedAt = time.Time{}
	result.ExpiresAt = time.Time{}
	result.Token = ""
	c.Assert(result, check.DeepEquals, authTypes.TeamToken{
		Team:         s.team.Name,
		TokenID:      "t1",
		CreatorEmail: s.token.GetUserName(),
		Description:  "desc",
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeTeam, Value: s.team.Name},
		Owner:  s.user.Email,
		Kind:   "team.token.create",
		StartCustomData: []map[string]interface{}{
			{"name": "token_id", "value": "t1"},
			{"name": "description", "value": "desc"},
			{"name": "expires_in", "value": "60"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestTeamTokenCreateNoPermission(c *check.C) {
	body := strings.NewReader(`token_id=t1&description=desc&expires_in=60&team=` + s.team.Name)
	request, err := http.NewRequest("POST", "/1.6/tokens", body)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermTeamTokenRead,
		Context: permission.Context(permTypes.CtxTeam, "teamx"),
	})
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestTeamTokenDelete(c *check.C) {
	_, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "id1",
	}, s.token)
	c.Assert(err, check.IsNil)

	request, err := http.NewRequest("DELETE", "/1.6/tokens/id1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	request, err = http.NewRequest("DELETE", "/1.6/tokens/id1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)

	_, err = servicemanager.TeamToken.FindByTokenID(context.TODO(), "id1")
	c.Assert(err, check.Equals, authTypes.ErrTeamTokenNotFound)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeTeam, Value: s.team.Name},
		Owner:  s.user.Email,
		Kind:   "team.token.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":token_id", "value": "id1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestTeamTokenDeleteNoPermission(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermTeamTokenDelete,
		Context: permission.Context(permTypes.CtxTeam, "otherteam"),
	})
	_, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "id1",
	}, s.token)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.6/tokens/id1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestTeamTokenUpdate(c *check.C) {
	originalToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "id1",
	}, s.token)
	c.Assert(err, check.IsNil)

	body := strings.NewReader(`regenerate=true`)
	request, err := http.NewRequest("PUT", "/1.6/tokens/id1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	newToken, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), "id1")
	c.Assert(err, check.IsNil)
	c.Assert(originalToken.Token, check.Not(check.Equals), newToken.Token)

	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeTeam, Value: s.team.Name},
		Owner:  s.user.Email,
		Kind:   "team.token.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":token_id", "value": "id1"},
			{"name": "regenerate", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestTeamTokenInfo(c *check.C) {
	newToken, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:        s.team.Name,
		Description: "desc",
		TokenID:     "id1",
	}, s.token)
	c.Assert(err, check.IsNil)

	request, err := http.NewRequest("GET", "/1.7/tokens/id1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	var result authTypes.TeamToken
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	newToken.CreatedAt = time.Unix(newToken.CreatedAt.Unix(), 0)
	result.CreatedAt = time.Unix(result.CreatedAt.Unix(), 0)
	c.Assert(newToken, check.DeepEquals, result)
}
