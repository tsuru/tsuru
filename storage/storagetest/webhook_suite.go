// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"
	"fmt"
	"sort"

	eventTypes "github.com/tsuru/tsuru/types/event"
	check "gopkg.in/check.v1"
)

type WebhookSuite struct {
	SuiteHooks
	WebhookStorage eventTypes.WebhookStorage
}

func (s *WebhookSuite) TestInsertWebhook(c *check.C) {
	w := eventTypes.Webhook{
		Name:      "wh1",
		TeamOwner: "team1",
		URL:       "http://mysrv.com:123/abc?a=b",
		Method:    "GET",
		EventFilter: eventTypes.WebhookEventFilter{
			TargetTypes:  []string{"app"},
			TargetValues: []string{"myapp"},
		},
	}
	err := s.WebhookStorage.Insert(context.TODO(), w)
	c.Assert(err, check.IsNil)
	webhook, err := s.WebhookStorage.FindByName(context.TODO(), w.Name)
	c.Assert(err, check.IsNil)
	c.Assert(webhook, check.DeepEquals, &eventTypes.Webhook{
		Name:      "wh1",
		TeamOwner: "team1",
		URL:       "http://mysrv.com:123/abc?a=b",
		Method:    "GET",
		EventFilter: eventTypes.WebhookEventFilter{
			KindTypes:    []string{},
			KindNames:    []string{},
			TargetTypes:  []string{"app"},
			TargetValues: []string{"myapp"},
		},
	})
}

func (s *WebhookSuite) TestFindByEvent(c *check.C) {
	filters := []eventTypes.WebhookEventFilter{
		{TargetTypes: []string{"app"}, TargetValues: []string{"myapp"}},
		{ErrorOnly: true},
		{SuccessOnly: true},
		{TargetTypes: []string{"app"}, TargetValues: []string{"myapp"}, ErrorOnly: true},
		{TargetTypes: []string{"app"}, TargetValues: []string{"myapp", "otherapp"}},
		{KindTypes: []string{"permission"}, KindNames: []string{"app.deploy", "app.update"}},
		{TargetTypes: []string{"node"}, KindNames: []string{"node.create", "healer"}},
		{TargetTypes: []string{"app"}, TargetValues: []string{"myapp"}, ErrorOnly: true, KindTypes: []string{"permission"}, KindNames: []string{"app.deploy", "app.update"}},
		{TargetTypes: []string{"app"}, TargetValues: []string{"otherapp"}},
		{KindTypes: []string{"permission"}, KindNames: []string{"app.update"}},
		{},
	}
	for i, f := range filters {
		w := eventTypes.Webhook{
			Name:        fmt.Sprintf("wh-%d", i),
			TeamOwner:   "team1",
			URL:         "http://mysrv.com:123/abc?a=b",
			Method:      "GET",
			EventFilter: f,
		}
		err := s.WebhookStorage.Insert(context.TODO(), w)
		c.Assert(err, check.IsNil)
	}
	tests := []struct {
		f        eventTypes.WebhookEventFilter
		success  bool
		expected []string
	}{
		{
			f: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"myapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.deploy"},
			},
			success:  false,
			expected: []string{"wh-0", "wh-1", "wh-3", "wh-4", "wh-5", "wh-7", "wh-10"},
		},
		{
			f: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"myapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.deploy"},
			},
			success:  true,
			expected: []string{"wh-0", "wh-2", "wh-4", "wh-5", "wh-10"},
		},
		{
			f: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"otherapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.deploy"},
			},
			success:  true,
			expected: []string{"wh-2", "wh-4", "wh-5", "wh-8", "wh-10"},
		},
		{
			f: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"anyapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.update.env.set"},
			},
			success:  true,
			expected: []string{"wh-2", "wh-5", "wh-9", "wh-10"},
		},
		{
			f: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{"node"},
				TargetValues: []string{"10.0.0.1", "10.0.0.2"},
				KindTypes:    []string{"internal"},
				KindNames:    []string{"healer"},
			},
			success:  true,
			expected: []string{"wh-2", "wh-6", "wh-10"},
		},
		{
			f: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"myapp", "otherapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.update"},
			},
			success:  true,
			expected: []string{"wh-0", "wh-2", "wh-4", "wh-5", "wh-8", "wh-9", "wh-10"},
		},
		{
			f: eventTypes.WebhookEventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"myapp", "otherapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.update"},
			},
			success:  false,
			expected: []string{"wh-0", "wh-1", "wh-3", "wh-4", "wh-5", "wh-7", "wh-8", "wh-9", "wh-10"},
		},
	}
	for i, tt := range tests {
		hooks, err := s.WebhookStorage.FindByEvent(context.TODO(), tt.f, tt.success)
		c.Assert(err, check.IsNil)
		names := webhooksNames(hooks)
		sort.Strings(tt.expected)
		c.Assert(names, check.DeepEquals, tt.expected, check.Commentf("failed test %d", i))
	}
}

func (s *WebhookSuite) TestInsertDuplicateWebhook(c *check.C) {
	t := eventTypes.Webhook{Name: "Webhookname"}
	err := s.WebhookStorage.Insert(context.TODO(), t)
	c.Assert(err, check.IsNil)
	err = s.WebhookStorage.Insert(context.TODO(), t)
	c.Assert(err, check.Equals, eventTypes.ErrWebhookAlreadyExists)
}

func webhooksNames(hooks []eventTypes.Webhook) []string {
	var names []string
	for _, h := range hooks {
		names = append(names, h.Name)
	}
	sort.Strings(names)
	return names
}

func (s *WebhookSuite) TestFindAllByTeams(c *check.C) {
	w1 := eventTypes.Webhook{Name: "wh1", TeamOwner: "t1"}
	err := s.WebhookStorage.Insert(context.TODO(), w1)
	c.Assert(err, check.IsNil)
	w2 := eventTypes.Webhook{Name: "wh2", TeamOwner: "t2"}
	err = s.WebhookStorage.Insert(context.TODO(), w2)
	c.Assert(err, check.IsNil)
	webhooks, err := s.WebhookStorage.FindAllByTeams(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(webhooksNames(webhooks), check.DeepEquals, []string{"wh1", "wh2"})
	webhooks, err = s.WebhookStorage.FindAllByTeams(context.TODO(), []string{})
	c.Assert(err, check.IsNil)
	c.Assert(webhooksNames(webhooks), check.IsNil)
	webhooks, err = s.WebhookStorage.FindAllByTeams(context.TODO(), []string{"t1"})
	c.Assert(err, check.IsNil)
	c.Assert(webhooksNames(webhooks), check.DeepEquals, []string{"wh1"})
	webhooks, err = s.WebhookStorage.FindAllByTeams(context.TODO(), []string{"t1", "t2"})
	c.Assert(err, check.IsNil)
	c.Assert(webhooksNames(webhooks), check.DeepEquals, []string{"wh1", "wh2"})
}

func (s *WebhookSuite) TestDelete(c *check.C) {
	w := eventTypes.Webhook{Name: "wh1"}
	err := s.WebhookStorage.Insert(context.TODO(), w)
	c.Assert(err, check.IsNil)
	err = s.WebhookStorage.Delete(context.TODO(), "wh1")
	c.Assert(err, check.IsNil)
	err = s.WebhookStorage.Delete(context.TODO(), "wh1")
	c.Assert(err, check.Equals, eventTypes.ErrWebhookNotFound)
	_, err = s.WebhookStorage.FindByName(context.TODO(), "wh1")
	c.Assert(err, check.Equals, eventTypes.ErrWebhookNotFound)
}

func (s *WebhookSuite) TestUpdate(c *check.C) {
	w := eventTypes.Webhook{Name: "wh1"}
	err := s.WebhookStorage.Insert(context.TODO(), w)
	c.Assert(err, check.IsNil)
	w.Method = "GET"
	err = s.WebhookStorage.Update(context.TODO(), w)
	c.Assert(err, check.IsNil)
	dbW, err := s.WebhookStorage.FindByName(context.TODO(), "wh1")
	c.Assert(err, check.IsNil)
	c.Assert(dbW, check.DeepEquals, &eventTypes.Webhook{
		Name:   "wh1",
		Method: "GET",
		EventFilter: eventTypes.WebhookEventFilter{
			KindTypes:    []string{},
			KindNames:    []string{},
			TargetTypes:  []string{},
			TargetValues: []string{},
		},
	})
}

func (s *WebhookSuite) TestUpdateNotFound(c *check.C) {
	err := s.WebhookStorage.Update(context.TODO(), eventTypes.Webhook{Name: "wh1"})
	c.Assert(err, check.Equals, eventTypes.ErrWebhookNotFound)
}

func (s *WebhookSuite) TestFindByNameNotFound(c *check.C) {
	_, err := s.WebhookStorage.FindByName(context.TODO(), "wh1")
	c.Assert(err, check.Equals, eventTypes.ErrWebhookNotFound)
}
