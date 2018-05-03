// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	eventTypes "github.com/tsuru/tsuru/types/event"
	check "gopkg.in/check.v1"
)

func (s *S) TestWebHookList(c *check.C) {
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	webHook2 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh2",
		URL:       "http://me",
	}
	err := servicemanager.WebHook.Create(webHook1)
	c.Assert(err, check.IsNil)
	err = servicemanager.WebHook.Create(webHook2)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/events/webhooks", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []eventTypes.WebHook
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []eventTypes.WebHook{
		{
			TeamOwner: s.team.Name,
			Name:      "wh1",
			URL:       "http://me",
			Headers:   http.Header{},
			EventFilter: eventTypes.WebHookEventFilter{
				TargetTypes:  []string{},
				TargetValues: []string{},
				KindTypes:    []string{},
				KindNames:    []string{"app.deploy"},
			},
		},
		{
			TeamOwner: s.team.Name,
			Name:      "wh2",
			URL:       "http://me",
			Headers:   http.Header{},
			EventFilter: eventTypes.WebHookEventFilter{
				TargetTypes:  []string{},
				TargetValues: []string{},
				KindTypes:    []string{},
				KindNames:    []string{},
			},
		},
	})
}

func (s *S) TestWebHookListEmpty(c *check.C) {
	request, err := http.NewRequest("GET", "/1.6/events/webhooks", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestWebHookListByTeam(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermWebhookRead,
		Context: permission.Context(permission.CtxTeam, "t2"),
	})
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	webHook2 := eventTypes.WebHook{
		TeamOwner: "t2",
		Name:      "wh2",
		URL:       "http://me",
	}
	err := servicemanager.WebHook.Create(webHook1)
	c.Assert(err, check.IsNil)
	err = servicemanager.WebHook.Create(webHook2)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/events/webhooks", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []eventTypes.WebHook
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []eventTypes.WebHook{
		{
			TeamOwner: "t2",
			Name:      "wh2",
			URL:       "http://me",
			Headers:   http.Header{},
			EventFilter: eventTypes.WebHookEventFilter{
				TargetTypes:  []string{},
				TargetValues: []string{},
				KindTypes:    []string{},
				KindNames:    []string{},
			},
		},
	})
}

func (s *S) TestWebHookCreate(c *check.C) {
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	bodyData, err := form.EncodeToString(webHook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/1.6/events/webhooks", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %s", recorder.Body.String()))
	wh, err := servicemanager.WebHook.Find("wh1")
	c.Assert(err, check.IsNil)
	c.Assert(wh, check.DeepEquals, eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		Headers:   http.Header{},
		EventFilter: eventTypes.WebHookEventFilter{
			TargetTypes:  []string{},
			TargetValues: []string{},
			KindTypes:    []string{},
			KindNames:    []string{"app.deploy"},
		},
	})
}

func (s *S) TestWebHookCreateConflict(c *check.C) {
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.WebHook.Create(webHook1)
	c.Assert(err, check.IsNil)
	bodyData, err := form.EncodeToString(webHook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/1.6/events/webhooks", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestWebHookCreateInvalid(c *check.C) {
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	bodyData, err := form.EncodeToString(webHook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/1.6/events/webhooks", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestWebHookUpdate(c *check.C) {
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.WebHook.Create(webHook1)
	c.Assert(err, check.IsNil)
	webHook1.Name = "---ignored---"
	webHook1.URL += "/xyz"
	bodyData, err := form.EncodeToString(webHook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.6/events/webhooks/wh1", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	wh, err := servicemanager.WebHook.Find("wh1")
	c.Assert(err, check.IsNil)
	c.Assert(wh, check.DeepEquals, eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me/xyz",
		Headers:   http.Header{},
		EventFilter: eventTypes.WebHookEventFilter{
			TargetTypes:  []string{},
			TargetValues: []string{},
			KindTypes:    []string{},
			KindNames:    []string{"app.deploy"},
		},
	})
}

func (s *S) TestWebHookUpdateNotFound(c *check.C) {
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	bodyData, err := form.EncodeToString(webHook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.6/events/webhooks/wh1", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestWebHookUpdateInvalid(c *check.C) {
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.WebHook.Create(webHook1)
	c.Assert(err, check.IsNil)
	webHook1.URL = ""
	bodyData, err := form.EncodeToString(webHook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.6/events/webhooks/wh1", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestWebHookDelete(c *check.C) {
	webHook1 := eventTypes.WebHook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebHookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.WebHook.Create(webHook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.6/events/webhooks/wh1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	_, err = servicemanager.WebHook.Find("wh1")
	c.Assert(err, check.Equals, eventTypes.ErrWebHookNotFound)
}

func (s *S) TestWebHookDeleteNotFound(c *check.C) {
	request, err := http.NewRequest("DELETE", "/1.6/events/webhooks/wh1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}
