// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cezarsa/form"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestWebhookList(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	webhook2 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh2",
		URL:       "http://me",
	}
	err := servicemanager.Webhook.Create(context.TODO(), webhook1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Webhook.Create(context.TODO(), webhook2)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/events/webhooks", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []eventTypes.Webhook
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []eventTypes.Webhook{
		{
			TeamOwner: s.team.Name,
			Name:      "wh1",
			URL:       "http://me",
			Headers:   http.Header{},
			EventFilter: eventTypes.WebhookEventFilter{
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
			EventFilter: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{},
				TargetValues: []string{},
				KindTypes:    []string{},
				KindNames:    []string{},
			},
		},
	})
}

func (s *S) TestWebhookListEmpty(c *check.C) {
	request, err := http.NewRequest("GET", "/1.6/events/webhooks", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestWebhookListByTeam(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermWebhookRead,
		Context: permission.Context(permTypes.CtxTeam, "t2"),
	})
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	webhook2 := eventTypes.Webhook{
		TeamOwner: "t2",
		Name:      "wh2",
		URL:       "http://me",
	}
	err := servicemanager.Webhook.Create(context.TODO(), webhook1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Webhook.Create(context.TODO(), webhook2)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/events/webhooks", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []eventTypes.Webhook
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []eventTypes.Webhook{
		{
			TeamOwner: "t2",
			Name:      "wh2",
			URL:       "http://me",
			Headers:   http.Header{},
			EventFilter: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{},
				TargetValues: []string{},
				KindTypes:    []string{},
				KindNames:    []string{},
			},
		},
	})
}

func (s *S) TestWebhookCreate(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	bodyData, err := form.EncodeToString(webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/1.6/events/webhooks", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %s", recorder.Body.String()))
	wh, err := servicemanager.Webhook.Find(context.TODO(), "wh1")
	c.Assert(err, check.IsNil)
	c.Assert(wh, check.DeepEquals, eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		Headers:   http.Header{},
		EventFilter: eventTypes.WebhookEventFilter{
			TargetTypes:  []string{},
			TargetValues: []string{},
			KindTypes:    []string{},
			KindNames:    []string{"app.deploy"},
		},
	})
}

func (s *S) TestWebhookCreateAutoTeam(c *check.C) {
	webhook1 := eventTypes.Webhook{
		Name: "wh1",
		URL:  "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	bodyData, err := form.EncodeToString(webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/1.6/events/webhooks", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %s", recorder.Body.String()))
	wh, err := servicemanager.Webhook.Find(context.TODO(), "wh1")
	c.Assert(err, check.IsNil)
	c.Assert(wh, check.DeepEquals, eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		Headers:   http.Header{},
		EventFilter: eventTypes.WebhookEventFilter{
			TargetTypes:  []string{},
			TargetValues: []string{},
			KindTypes:    []string{},
			KindNames:    []string{"app.deploy"},
		},
	})
}

func (s *S) TestWebhookCreateConflict(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.Webhook.Create(context.TODO(), webhook1)
	c.Assert(err, check.IsNil)
	bodyData, err := form.EncodeToString(webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/1.6/events/webhooks", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestWebhookCreateInvalid(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	bodyData, err := form.EncodeToString(webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/1.6/events/webhooks", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestWebhookUpdate(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.Webhook.Create(context.TODO(), webhook1)
	c.Assert(err, check.IsNil)
	webhook1.Name = "---ignored---"
	webhook1.URL += "/xyz"
	bodyData, err := form.EncodeToString(webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.6/events/webhooks/wh1", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	wh, err := servicemanager.Webhook.Find(context.TODO(), "wh1")
	c.Assert(err, check.IsNil)
	c.Assert(wh, check.DeepEquals, eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me/xyz",
		Headers:   http.Header{},
		EventFilter: eventTypes.WebhookEventFilter{
			TargetTypes:  []string{},
			TargetValues: []string{},
			KindTypes:    []string{},
			KindNames:    []string{"app.deploy"},
		},
	})
}

func (s *S) TestWebhookUpdateNotFound(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	bodyData, err := form.EncodeToString(webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.6/events/webhooks/wh1", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestWebhookUpdateInvalid(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.Webhook.Create(context.TODO(), webhook1)
	c.Assert(err, check.IsNil)
	webhook1.URL = ""
	bodyData, err := form.EncodeToString(webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.6/events/webhooks/wh1", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestWebhookDelete(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.Webhook.Create(context.TODO(), webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.6/events/webhooks/wh1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	_, err = servicemanager.Webhook.Find(context.TODO(), "wh1")
	c.Assert(err, check.Equals, eventTypes.ErrWebhookNotFound)
}

func (s *S) TestWebhookDeleteNotFound(c *check.C) {
	request, err := http.NewRequest("DELETE", "/1.6/events/webhooks/wh1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestWebhookInfo(c *check.C) {
	webhook1 := eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me/xyz",
		EventFilter: eventTypes.WebhookEventFilter{
			KindNames: []string{"app.deploy"},
		},
	}
	err := servicemanager.Webhook.Create(context.TODO(), webhook1)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.6/events/webhooks/wh1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var wh eventTypes.Webhook
	err = json.Unmarshal(recorder.Body.Bytes(), &wh)
	c.Assert(err, check.IsNil)
	c.Assert(wh, check.DeepEquals, eventTypes.Webhook{
		TeamOwner: s.team.Name,
		Name:      "wh1",
		URL:       "http://me/xyz",
		Headers:   http.Header{},
		EventFilter: eventTypes.WebhookEventFilter{
			TargetTypes:  []string{},
			TargetValues: []string{},
			KindTypes:    []string{},
			KindNames:    []string{"app.deploy"},
		},
	})
}

func (s *S) TestWebhookInfoNotFound(c *check.C) {
	request, err := http.NewRequest("GET", "/1.6/events/webhooks/wh1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}
