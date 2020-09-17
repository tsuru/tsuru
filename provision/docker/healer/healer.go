// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/types"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

var (
	consecutiveHealingsTimeframe        = 5 * time.Minute
	consecutiveHealingsLimitInTimeframe = 3
)

func init() {
	event.SetThrottling(event.ThrottlingSpec{
		TargetType: event.TargetTypeContainer,
		KindName:   "healer",
		Time:       consecutiveHealingsTimeframe,
		Max:        consecutiveHealingsLimitInTimeframe,
		AllTargets: true,
		WaitFinish: true,
	})
}

func toHealingEvt(evt *event.Event) (types.HealingEvent, error) {
	healingEvt := types.HealingEvent{
		ID:         evt.UniqueID,
		StartTime:  evt.StartTime,
		EndTime:    evt.EndTime,
		Action:     fmt.Sprintf("%s-healing", evt.Target.Type),
		Successful: evt.Error == "",
		Error:      evt.Error,
	}
	switch evt.Target.Type {
	case event.TargetTypeContainer:
		err := evt.StartData(&healingEvt.FailingContainer)
		if err != nil {
			return healingEvt, err
		}
		err = evt.EndData(&healingEvt.CreatedContainer)
		if err != nil {
			return healingEvt, err
		}
	case event.TargetTypeNode:
		var data healer.NodeHealerCustomData
		err := evt.StartData(&data)
		if err != nil {
			return healingEvt, err
		}
		if data.LastCheck != nil {
			healingEvt.Extra = data.LastCheck
		}
		healingEvt.Reason = data.Reason
		healingEvt.FailingNode = data.Node
		var createdNode provision.NodeSpec
		err = evt.EndData(&createdNode)
		if err != nil {
			return healingEvt, err
		}
		healingEvt.CreatedNode = createdNode
	}

	return healingEvt, nil
}

func ListHealingHistory(filter string) ([]types.HealingEvent, error) {
	evtFilter := event.Filter{
		KindNames: []string{"healer"},
		KindType:  event.KindTypeInternal,
	}
	if filter != "" {
		t, err := event.GetTargetType(filter)
		if err == nil {
			evtFilter.Target = event.Target{Type: t}
		}
	}
	evts, err := event.List(&evtFilter)
	if err != nil {
		return nil, err
	}
	healingEvts := make([]types.HealingEvent, len(evts))
	for i := range evts {
		healingEvts[i], err = toHealingEvt(evts[i])
		if err != nil {
			return nil, err
		}
	}
	return healingEvts, nil
}

func oldHealingCollection() (*storage.Collection, error) {
	name, _ := config.GetString("docker:healing:events_collection")
	if name == "" {
		name = "healing_events"
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection(name), nil
}

func healingEventToEvent(data *types.HealingEvent) error {
	var evt event.Event
	evt.UniqueID = data.ID.(bson.ObjectId)
	var startOpts, endOpts interface{}
	switch data.Action {
	case "node-healing":
		evt.Target = event.Target{Type: event.TargetTypeNode, Value: data.FailingNode.Address}
		var lastCheck *healer.NodeChecks
		if data.Extra != nil {
			checkRaw, err := json.Marshal(data.Extra)
			if err == nil {
				json.Unmarshal(checkRaw, &lastCheck)
			}
		}
		startOpts = healer.NodeHealerCustomData{
			Node:      data.FailingNode,
			Reason:    data.Reason,
			LastCheck: lastCheck,
		}
		endOpts = data.CreatedNode
		poolName := data.FailingNode.Metadata[provision.PoolMetadataName]
		evt.Allowed = event.Allowed(permission.PermPoolReadEvents, permission.Context(permTypes.CtxPool, poolName))
	case "container-healing":
		evt.Target = event.Target{Type: event.TargetTypeContainer, Value: data.FailingContainer.ID}
		startOpts = data.FailingContainer
		endOpts = data.CreatedContainer
		a, err := app.GetByName(context.TODO(), data.FailingContainer.AppName)
		if err == nil {
			evt.Allowed = event.Allowed(permission.PermAppReadEvents, append(permission.Contexts(permTypes.CtxTeam, a.Teams),
				permission.Context(permTypes.CtxApp, a.Name),
				permission.Context(permTypes.CtxPool, a.Pool),
			)...)
		} else {
			evt.Allowed = event.Allowed(permission.PermAppReadEvents)
		}
	default:
		return errors.Errorf("invalid action %q", data.Action)
	}
	evt.Owner = event.Owner{Type: event.OwnerTypeInternal}
	evt.Kind = event.Kind{Type: event.KindTypeInternal, Name: "healer"}
	evt.StartTime = data.StartTime
	evt.EndTime = data.EndTime
	evt.Error = data.Error
	err := evt.RawInsert(startOpts, nil, endOpts)
	if mgo.IsDup(err) {
		return nil
	}
	return err
}

func MigrateHealingToEvents() error {
	coll, err := oldHealingCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	coll.Find(nil).Iter()
	iter := coll.Find(nil).Iter()
	var data types.HealingEvent
	for iter.Next(&data) {
		err = healingEventToEvent(&data)
		if err != nil {
			return err
		}
	}
	return iter.Close()
}
