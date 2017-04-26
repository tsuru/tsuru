// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	check "gopkg.in/check.v1"
)

func (s *S) TestHealingHistoryNoContent(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestHealingHistoryHandler(c *check.C) {
	evt1, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeNode, Value: "addr1"},
		InternalKind: "healer",
		CustomData:   map[string]interface{}{"node": cluster.Node{Address: "addr1"}},
		Allowed:      event.Allowed(permission.PermPool),
	})
	c.Assert(err, check.IsNil)
	evt1.DoneCustomData(nil, cluster.Node{Address: "addr2"})
	time.Sleep(10 * time.Millisecond)
	evt2, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeNode, Value: "addr3"},
		InternalKind: "healer",
		CustomData:   map[string]interface{}{"node": cluster.Node{Address: "addr3"}},
		Allowed:      event.Allowed(permission.PermPool),
	})
	c.Assert(err, check.IsNil)
	evt2.DoneCustomData(errors.New("some error"), cluster.Node{})
	time.Sleep(10 * time.Millisecond)
	evt3, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeContainer, Value: "1234"},
		InternalKind: "healer",
		CustomData:   container.Container{Container: types.Container{ID: "1234"}},
		Allowed:      event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	evt3.DoneCustomData(nil, container.Container{Container: types.Container{ID: "9876"}})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	body := recorder.Body.Bytes()
	var healings []types.HealingEvent
	err = json.Unmarshal(body, &healings)
	c.Assert(err, check.IsNil)
	c.Assert(healings, check.HasLen, 3)
	c.Assert(healings[2].StartTime.UTC().Format(time.Stamp), check.Equals, evt1.StartTime.UTC().Format(time.Stamp))
	c.Assert(healings[2].EndTime.UTC().Format(time.Stamp), check.Equals, evt1.EndTime.UTC().Format(time.Stamp))
	c.Assert(healings[2].FailingNode.Address, check.Equals, "addr1")
	c.Assert(healings[2].CreatedNode.Address, check.Equals, "addr2")
	c.Assert(healings[2].Error, check.Equals, "")
	c.Assert(healings[2].Successful, check.Equals, true)
	c.Assert(healings[2].Action, check.Equals, "node-healing")
	c.Assert(healings[1].FailingNode.Address, check.Equals, "addr3")
	c.Assert(healings[1].CreatedNode.Address, check.Equals, "")
	c.Assert(healings[1].Error, check.Equals, "some error")
	c.Assert(healings[1].Successful, check.Equals, false)
	c.Assert(healings[1].Action, check.Equals, "node-healing")
	c.Assert(healings[0].FailingContainer.ID, check.Equals, "1234")
	c.Assert(healings[0].CreatedContainer.ID, check.Equals, "9876")
	c.Assert(healings[0].Successful, check.Equals, true)
	c.Assert(healings[0].Error, check.Equals, "")
	c.Assert(healings[0].Action, check.Equals, "container-healing")
}

func (s *S) TestHealingHistoryHandlerFilterContainer(c *check.C) {
	evt1, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeNode, Value: "addr1"},
		InternalKind: "healer",
		CustomData:   map[string]interface{}{"node": cluster.Node{Address: "addr1"}},
		Allowed:      event.Allowed(permission.PermPool),
	})
	c.Assert(err, check.IsNil)
	evt1.DoneCustomData(nil, cluster.Node{Address: "addr2"})
	time.Sleep(10 * time.Millisecond)
	evt2, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeNode, Value: "addr3"},
		InternalKind: "healer",
		CustomData:   map[string]interface{}{"node": cluster.Node{Address: "addr3"}},
		Allowed:      event.Allowed(permission.PermPool),
	})
	c.Assert(err, check.IsNil)
	evt2.DoneCustomData(errors.New("some error"), cluster.Node{})
	time.Sleep(10 * time.Millisecond)
	evt3, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeContainer, Value: "1234"},
		InternalKind: "healer",
		CustomData:   container.Container{Container: types.Container{ID: "1234"}},
		Allowed:      event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	evt3.DoneCustomData(nil, container.Container{Container: types.Container{ID: "9876"}})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing?filter=container", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	var healings []types.HealingEvent
	err = json.Unmarshal(body, &healings)
	c.Assert(err, check.IsNil)
	c.Assert(healings, check.HasLen, 1)
	c.Assert(healings[0].FailingContainer.ID, check.Equals, "1234")
	c.Assert(healings[0].CreatedContainer.ID, check.Equals, "9876")
	c.Assert(healings[0].Successful, check.Equals, true)
	c.Assert(healings[0].Error, check.Equals, "")
	c.Assert(healings[0].Action, check.Equals, "container-healing")
}

func (s *S) TestHealingHistoryHandlerFilterNode(c *check.C) {
	evt1, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: "node", Value: "addr1"},
		InternalKind: "healer",
		CustomData:   map[string]interface{}{"node": cluster.Node{Address: "addr1"}},
		Allowed:      event.Allowed(permission.PermPool),
	})
	c.Assert(err, check.IsNil)
	evt1.DoneCustomData(nil, cluster.Node{Address: "addr2"})
	time.Sleep(10 * time.Millisecond)
	evt2, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: "node", Value: "addr3"},
		InternalKind: "healer",
		CustomData:   map[string]interface{}{"node": cluster.Node{Address: "addr3"}},
		Allowed:      event.Allowed(permission.PermPool),
	})
	c.Assert(err, check.IsNil)
	evt2.DoneCustomData(errors.New("some error"), cluster.Node{})
	time.Sleep(10 * time.Millisecond)
	evt3, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: "container", Value: "1234"},
		InternalKind: "healer",
		CustomData:   container.Container{Container: types.Container{ID: "1234"}},
		Allowed:      event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	evt3.DoneCustomData(nil, container.Container{Container: types.Container{ID: "9876"}})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/docker/healing?filter=node", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	var healings []types.HealingEvent
	err = json.Unmarshal(body, &healings)
	c.Assert(err, check.IsNil)
	c.Assert(healings, check.HasLen, 2)
	c.Assert(healings[0].Action, check.Equals, "node-healing")
	c.Assert(healings[0].ID, check.Equals, evt2.UniqueID.Hex())
	c.Assert(healings[0].FailingNode.Address, check.Equals, "addr3")
	c.Assert(healings[1].Action, check.Equals, "node-healing")
	c.Assert(healings[1].ID, check.Equals, evt1.UniqueID.Hex())
	c.Assert(healings[1].FailingNode.Address, check.Equals, "addr1")
}
