// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/tsuru/tsuru/permission"
	eventTypes "github.com/tsuru/tsuru/types/event"
	"gopkg.in/check.v1"
)

type WebHookSuite struct {
	SuiteHooks
	WebHookStorage eventTypes.WebHookStorage
}

func (s *WebHookSuite) TestInsertWebHook(c *check.C) {
	u, _ := url.Parse("http://mysrv.com:123/abc?a=b")
	w := eventTypes.WebHook{
		Name:      "wh1",
		TeamOwner: "team1",
		URL:       *u,
		Method:    "GET",
		EventFilter: eventTypes.EventFilter{
			TargetTypes:  []string{"app"},
			TargetValues: []string{"myapp"},
		},
	}
	err := s.WebHookStorage.Insert(w)
	c.Assert(err, check.IsNil)
	webHook, err := s.WebHookStorage.FindByName(w.Name)
	c.Assert(err, check.IsNil)
	c.Assert(webHook, check.DeepEquals, &eventTypes.WebHook{
		Name:      "wh1",
		TeamOwner: "team1",
		URL:       *u,
		Method:    "GET",
		Headers:   http.Header{},
		EventFilter: eventTypes.EventFilter{
			KindTypes:    []string{},
			KindNames:    []string{},
			TargetTypes:  []string{"app"},
			TargetValues: []string{"myapp"},
			AllowedPermissions: eventTypes.WebHookAllowedPermission{
				Contexts: []permission.PermissionContext{},
			},
		},
	})
}

func (s *WebHookSuite) TestFindByEvent(c *check.C) {
	filters := []eventTypes.EventFilter{
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
	}
	u, _ := url.Parse("http://mysrv.com:123/abc?a=b")
	for i, f := range filters {
		w := eventTypes.WebHook{
			Name:        fmt.Sprintf("wh-%d", i),
			TeamOwner:   "team1",
			URL:         *u,
			Method:      "GET",
			EventFilter: f,
		}
		err := s.WebHookStorage.Insert(w)
		c.Assert(err, check.IsNil)
	}
	tests := []struct {
		f        eventTypes.EventFilter
		success  bool
		expected []string
	}{
		{
			f: eventTypes.EventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"myapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.deploy"},
			},
			success:  false,
			expected: []string{"wh-0", "wh-1", "wh-3", "wh-4", "wh-5", "wh-7"},
		},
		{
			f: eventTypes.EventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"myapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.deploy"},
			},
			success:  true,
			expected: []string{"wh-0", "wh-2", "wh-4", "wh-5"},
		},
		{
			f: eventTypes.EventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"otherapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.deploy"},
			},
			success:  true,
			expected: []string{"wh-2", "wh-4", "wh-5", "wh-8"},
		},
		{
			f: eventTypes.EventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"anyapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.update.env.set"},
			},
			success:  true,
			expected: []string{"wh-2", "wh-5", "wh-9"},
		},
		{
			f: eventTypes.EventFilter{
				TargetTypes:  []string{"node"},
				TargetValues: []string{"10.0.0.1", "10.0.0.2"},
				KindTypes:    []string{"internal"},
				KindNames:    []string{"healer"},
			},
			success:  true,
			expected: []string{"wh-2", "wh-6"},
		},
		{
			f: eventTypes.EventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"myapp", "otherapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.update"},
			},
			success:  true,
			expected: []string{"wh-0", "wh-2", "wh-4", "wh-5", "wh-8", "wh-9"},
		},
		{
			f: eventTypes.EventFilter{
				TargetTypes:  []string{"app"},
				TargetValues: []string{"myapp", "otherapp"},
				KindTypes:    []string{"permission"},
				KindNames:    []string{"app.update"},
			},
			success:  false,
			expected: []string{"wh-0", "wh-1", "wh-3", "wh-4", "wh-5", "wh-7", "wh-8", "wh-9"},
		},
	}
	for i, tt := range tests {
		hooks, err := s.WebHookStorage.FindByEvent(tt.f, tt.success)
		c.Assert(err, check.IsNil)
		names := webhooksNames(hooks)
		c.Assert(names, check.DeepEquals, tt.expected, check.Commentf("failed test %d", i))
	}
}

func (s *WebHookSuite) TestInsertDuplicateWebHook(c *check.C) {
	t := eventTypes.WebHook{Name: "WebHookname"}
	err := s.WebHookStorage.Insert(t)
	c.Assert(err, check.IsNil)
	err = s.WebHookStorage.Insert(t)
	c.Assert(err, check.Equals, eventTypes.ErrWebHookAlreadyExists)
}

func webhooksNames(hooks []eventTypes.WebHook) []string {
	var names []string
	for _, h := range hooks {
		names = append(names, h.Name)
	}
	sort.Strings(names)
	return names
}

func (s *WebHookSuite) TestFindAllByTeams(c *check.C) {
	w1 := eventTypes.WebHook{Name: "wh1", TeamOwner: "t1"}
	err := s.WebHookStorage.Insert(w1)
	c.Assert(err, check.IsNil)
	w2 := eventTypes.WebHook{Name: "wh2", TeamOwner: "t2"}
	err = s.WebHookStorage.Insert(w2)
	c.Assert(err, check.IsNil)
	webhooks, err := s.WebHookStorage.FindAllByTeams(nil)
	c.Assert(err, check.IsNil)
	c.Assert(webhooksNames(webhooks), check.DeepEquals, []string{"wh1", "wh2"})
	webhooks, err = s.WebHookStorage.FindAllByTeams([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(webhooksNames(webhooks), check.IsNil)
	webhooks, err = s.WebHookStorage.FindAllByTeams([]string{"t1"})
	c.Assert(err, check.IsNil)
	c.Assert(webhooksNames(webhooks), check.DeepEquals, []string{"wh1"})
	webhooks, err = s.WebHookStorage.FindAllByTeams([]string{"t1", "t2"})
	c.Assert(err, check.IsNil)
	c.Assert(webhooksNames(webhooks), check.DeepEquals, []string{"wh1", "wh2"})
}

func (s *WebHookSuite) TestDelete(c *check.C) {
	w := eventTypes.WebHook{Name: "wh1"}
	err := s.WebHookStorage.Insert(w)
	c.Assert(err, check.IsNil)
	err = s.WebHookStorage.Delete("wh1")
	c.Assert(err, check.IsNil)
	err = s.WebHookStorage.Delete("wh1")
	c.Assert(err, check.Equals, eventTypes.ErrWebHookNotFound)
	_, err = s.WebHookStorage.FindByName("wh1")
	c.Assert(err, check.Equals, eventTypes.ErrWebHookNotFound)
}

func (s *WebHookSuite) TestUpdate(c *check.C) {
	w := eventTypes.WebHook{Name: "wh1"}
	err := s.WebHookStorage.Insert(w)
	c.Assert(err, check.IsNil)
	w.Method = "GET"
	err = s.WebHookStorage.Update(w)
	c.Assert(err, check.IsNil)
	dbW, err := s.WebHookStorage.FindByName("wh1")
	c.Assert(err, check.IsNil)
	c.Assert(dbW, check.DeepEquals, &eventTypes.WebHook{
		Name:    "wh1",
		Method:  "GET",
		Headers: http.Header{},
		EventFilter: eventTypes.EventFilter{
			KindTypes:    []string{},
			KindNames:    []string{},
			TargetTypes:  []string{},
			TargetValues: []string{},
			AllowedPermissions: eventTypes.WebHookAllowedPermission{
				Contexts: []permission.PermissionContext{},
			},
		},
	})
}

func (s *WebHookSuite) TestUpdateNotFound(c *check.C) {
	err := s.WebHookStorage.Update(eventTypes.WebHook{Name: "wh1"})
	c.Assert(err, check.Equals, eventTypes.ErrWebHookNotFound)
}

func (s *WebHookSuite) TestFindByNameNotFound(c *check.C) {
	_, err := s.WebHookStorage.FindByName("wh1")
	c.Assert(err, check.Equals, eventTypes.ErrWebHookNotFound)
}
