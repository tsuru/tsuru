// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"gopkg.in/mgo.v2/bson"
)

// title: event list
// path: /events
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func eventList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	filter := &event.Filter{}
	if target := r.URL.Query().Get("target"); target != "" {
		t, err := event.GetTargetType(target)
		if err == nil {
			filter.Target = event.Target{Type: t}
		}
	}
	if running, err := strconv.ParseBool(r.URL.Query().Get("running")); err == nil {
		filter.Running = &running
	}
	if kindName := r.URL.Query().Get("kindName"); kindName != "" {
		filter.KindName = kindName
	}
	events, err := event.List(filter)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(events)
}

// title: kind list
// path: /events/kinds
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func kindList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	kinds, err := event.GetKinds()
	if err != nil {
		return err
	}
	if len(kinds) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(kinds)
}

// title: event info
// path: /events/{uuid}
// method: GET
// produce: application/json
// responses:
//   200: OK
//   400: Invalid uuid
//   401: Unauthorized
//   404: Not found
func eventInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	uuid := r.URL.Query().Get(":uuid")
	if !bson.IsObjectIdHex(uuid) {
		msg := fmt.Sprintf("uuid parameter is not ObjectId: %s", uuid)
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	objID := bson.ObjectIdHex(uuid)
	e, err := event.GetByID(objID)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	var hasPermission bool
	if e.Target.Type == event.TargetTypeApp {
		a, err := getAppFromContext(e.Target.Value, r)
		if err != nil {
			return err
		}
		hasPermission = permission.Check(t, permission.PermAppReadEvents,
			append(permission.Contexts(permission.CtxTeam, a.Teams),
				permission.Context(permission.CtxApp, a.Name),
				permission.Context(permission.CtxPool, a.Pool),
			)...,
		)
	}
	if e.Target.Type == event.TargetTypeTeam {
		tm, err := auth.GetTeam(e.Target.Value)
		if err != nil {
			return err
		}
		hasPermission = permission.Check(
			t, permission.PermTeamReadEvents,
			permission.Context(permission.CtxTeam, tm.Name),
		)
	}
	if e.Target.Type == event.TargetTypeService {
		s, err := getService(e.Target.Value)
		if err != nil {
			return err
		}
		hasPermission = permission.Check(t, permission.PermServiceReadEvents,
			append(permission.Contexts(permission.CtxTeam, s.OwnerTeams),
				permission.Context(permission.CtxService, s.Name),
			)...,
		)
	}
	if e.Target.Type == event.TargetTypeServiceInstance {
		if v := strings.SplitN(e.Target.Value, "_", 2); len(v) == 2 {
			si, err := getServiceInstanceOrError(v[0], v[1])
			if err != nil {
				return err
			}
			permissionValue := v[0] + "/" + v[1]
			hasPermission = permission.Check(t, permission.PermServiceInstanceReadEvents,
				append(permission.Contexts(permission.CtxTeam, si.Teams),
					permission.Context(permission.CtxServiceInstance, permissionValue),
				)...,
			)
		}
	}
	if !hasPermission {
		return permission.ErrUnauthorized
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(e)
}

// title: event cancel
// path: /events/{uuid}/cancel
// method: POST
// produce: application/json
// responses:
//   200: OK
//   400: Invalid uuid or empty reason
//   404: Not found
func eventCancel(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	uuid := r.URL.Query().Get(":uuid")
	if !bson.IsObjectIdHex(uuid) {
		msg := fmt.Sprintf("uuid parameter is not ObjectId: %s", uuid)
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	objID := bson.ObjectIdHex(uuid)
	e, err := event.GetByID(objID)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	reason := r.FormValue("reason")
	if reason == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "reason is mandatory"}
	}
	err = e.TryCancel(reason, t.GetUserName())
	if err != nil {
		if err == event.ErrNotCancelable {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
