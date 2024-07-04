// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eventtest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	eventTypes "github.com/tsuru/tsuru/types/event"
	check "gopkg.in/check.v1"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

type EventDesc struct {
	Target          eventTypes.Target
	ExtraTargets    []eventTypes.ExtraTarget
	Kind            string
	Owner           string
	StartCustomData interface{}
	EndCustomData   interface{}
	OtherCustomData interface{}
	LogMatches      []string
	ErrorMatches    string
	IsEmpty         bool
}

type hasEventChecker struct{}

func (hasEventChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "HasEvent", Params: []string{"event desc"}}
}

func queryPartCustom(query map[string]interface{}, name string, value interface{}) {
	if value == nil {
		return
	}
	switch data := value.(type) {
	case map[string]interface{}:
		for k, v := range data {
			query[name+"."+k] = v
		}
	case []map[string]interface{}:
		queryPart := []mongoBSON.M{}
		for _, el := range data {
			queryPart = append(queryPart, mongoBSON.M{
				name: mongoBSON.M{"$elemMatch": el},
			})
		}
		addAndBlock(query, queryPart)
	}
}

func (hasEventChecker) Check(params []interface{}, names []string) (bool, string) {
	ctx := context.TODO()
	var evt EventDesc
	switch params[0].(type) {
	case EventDesc:
		evt = params[0].(EventDesc)
	case *EventDesc:
		evt = *params[0].(*EventDesc)
	default:
		return false, "First parameter must be of type EventDesc or *EventDesc"
	}
	collection, err := storagev2.EventsCollection()
	if err != nil {
		return false, err.Error()
	}
	if evt.IsEmpty {
		var n int64

		n, err = collection.CountDocuments(ctx, mongoBSON.M{})
		if err != nil {
			return false, err.Error()
		}
		if n != 0 {
			return false, fmt.Sprintf("expected 0 events, got %d", n)
		}
		return true, ""
	}
	query := mongoBSON.M{
		"target":     evt.Target,
		"kind.name":  evt.Kind,
		"owner.name": evt.Owner,
		"running":    false,
	}
	if len(evt.ExtraTargets) > 0 {
		var andBlock []mongoBSON.M
		for _, t := range evt.ExtraTargets {
			andBlock = append(andBlock, mongoBSON.M{
				"extratargets": t,
			})
		}
		addAndBlock(query, andBlock)
	}
	queryPartCustom(query, "startcustomdata", evt.StartCustomData)
	queryPartCustom(query, "endcustomdata", evt.EndCustomData)
	queryPartCustom(query, "othercustomdata", evt.OtherCustomData)
	if len(evt.LogMatches) > 0 {
		var andBlock []mongoBSON.M
		for _, m := range evt.LogMatches {
			andBlock = append(andBlock, mongoBSON.M{
				"structuredlog.message": mongoBSON.M{"$regex": m},
			})
		}
		addAndBlock(query, andBlock)
	}

	if evt.ErrorMatches != "" {
		query["error"] = mongoBSON.M{"$regex": evt.ErrorMatches, "$options": "s"}
	} else {
		query["error"] = ""
	}
	n, err := collection.CountDocuments(ctx, query)
	if err != nil {
		return false, err.Error()
	}
	if n == 0 {
		all, _ := event.All(context.TODO())
		msg := fmt.Sprintf("Event not found. Existing events in DB: %s", debugEvts(all))
		return false, msg
	}
	if n > 1 {
		return false, "Multiple events match query"
	}
	return true, ""
}

func debugEvts(evts []*event.Event) string {
	var msgs []string
	for i := range evts {
		evt := evts[i]
		var sData, oData, eData interface{}
		evt.StartData(&sData)
		evt.OtherData(&oData)
		evt.EndData(&eData)
		evt.StartCustomData = mongoBSON.RawValue{}
		evt.OtherCustomData = mongoBSON.RawValue{}
		evt.EndCustomData = mongoBSON.RawValue{}
		msgs = append(msgs, fmt.Sprintf("%#v\nstartData: %#v\notherData: %#v\nendData: %#v", evt, sData, oData, eData))
	}
	return strings.Join(msgs, "\n****\n")
}

var HasEvent check.Checker = hasEventChecker{}

type evtEqualsChecker struct {
	check.CheckerInfo
}

func (evtEqualsChecker) Check(params []interface{}, names []string) (bool, string) {
	evts := make([][]*event.Event, len(params))
	for i := range evts {
		switch e := params[i].(type) {
		case event.Event:
			evts[i] = []*event.Event{&e}
		case *event.Event:
			evts[i] = []*event.Event{e}
		case []event.Event:
			for j := range e {
				evts[i] = append(evts[i], &e[j])
			}
		case []*event.Event:
			evts[i] = append(evts[i], e...)
		default:
			evts[i] = nil
		}
		for j := range evts[i] {
			e := evts[i][j]
			e.StartTime = time.Time{}
			e.EndTime = time.Time{}
			e.LockUpdateTime = time.Time{}
		}
	}
	return check.DeepEquals.Check([]interface{}{evts[0], evts[1]}, names)
}

var EvtEquals check.Checker = &evtEqualsChecker{
	check.CheckerInfo{Name: "EvtEquals", Params: []string{"obtained", "expected"}},
}

func addAndBlock(query mongoBSON.M, parts []mongoBSON.M) {
	if len(parts) == 0 {
		return
	}
	andBlock, ok := query["$and"].([]mongoBSON.M)
	if !ok {
		andBlock = []mongoBSON.M{}
	}
	andBlock = append(andBlock, parts...)
	query["$and"] = andBlock
}
