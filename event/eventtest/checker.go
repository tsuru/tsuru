// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eventtest

import (
	"fmt"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/event"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type EventDesc struct {
	Target          event.Target
	Kind            string
	Owner           string
	StartCustomData interface{}
	EndCustomData   interface{}
	LogMatches      string
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
		queryPart := []bson.M{}
		for _, el := range data {
			queryPart = append(queryPart, bson.M{
				name: bson.M{"$elemMatch": el},
			})
		}
		query["$and"] = queryPart
	}
}

func (hasEventChecker) Check(params []interface{}, names []string) (bool, string) {
	var evt EventDesc
	switch params[0].(type) {
	case EventDesc:
		evt = params[0].(EventDesc)
	case *EventDesc:
		evt = *params[0].(*EventDesc)
	default:
		return false, "First parameter must be of type EventDesc or *EventDesc"
	}
	conn, err := db.Conn()
	if err != nil {
		return false, err.Error()
	}
	defer conn.Close()
	if evt.IsEmpty {
		n, err := conn.Events().Find(nil).Count()
		if err != nil {
			return false, err.Error()
		}
		if n != 0 {
			return false, fmt.Sprintf("expected 0 events, got %d", n)
		}
		return true, ""
	}
	query := map[string]interface{}{
		"target":     evt.Target,
		"kind.name":  evt.Kind,
		"owner.name": evt.Owner,
		"running":    false,
	}
	queryPartCustom(query, "startcustomdata", evt.StartCustomData)
	queryPartCustom(query, "endcustomdata", evt.EndCustomData)
	if evt.LogMatches != "" {
		query["log"] = bson.M{"$regex": evt.LogMatches}
	}
	if evt.ErrorMatches != "" {
		query["error"] = bson.M{"$regex": evt.ErrorMatches}
	} else {
		query["error"] = ""
	}
	n, err := conn.Events().Find(query).Count()
	if err != nil {
		return false, err.Error()
	}
	if n == 0 {
		all, _ := event.All()
		msg := fmt.Sprintf("Event not found. Existing events in DB: %#v", all)
		return false, msg
	}
	if n > 1 {
		return false, "Multiple events match query"
	}
	return true, ""
}

var HasEvent check.Checker = hasEventChecker{}
