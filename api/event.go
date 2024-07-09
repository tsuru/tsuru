// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	eventTypes "github.com/tsuru/tsuru/types/event"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

// title: event list
// path: /events
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	204: No content
func eventList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	var filter *event.Filter
	err := ParseInput(r, &filter)
	if err != nil {
		return err
	}
	filter.LoadKindNames(r.Form)
	filter.PruneUserValues()
	filter.Permissions, err = t.Permissions(ctx)
	if err != nil {
		return err
	}
	events, err := event.List(ctx, filter)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	for _, event := range events {
		err = suppressSensitiveEnvs(event)
		if err != nil {
			return err
		}
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(events)
}

// title: kind list
// path: /events/kinds
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	204: No content
func kindList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	kinds, err := event.GetKinds(ctx)
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
//
//	200: OK
//	400: Invalid uuid
//	401: Unauthorized
//	404: Not found
func eventInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	uuid := r.URL.Query().Get(":uuid")
	if !bson.IsObjectIdHex(uuid) {
		msg := fmt.Sprintf("uuid parameter is not ObjectId: %s", uuid)
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	e, err := event.GetByHexID(ctx, uuid)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	scheme, err := permission.SafeGet(e.Allowed.Scheme)
	if err != nil {
		return err
	}
	allowed := permission.Check(ctx, t, scheme, e.Allowed.Contexts...)
	if !allowed {
		return permission.ErrUnauthorized
	}
	w.Header().Add("Content-Type", "application/json")
	err = suppressSensitiveEnvs(e)
	if err != nil {
		return err
	}

	eventInfo, err := event.EventInfo(e)
	if err != nil {
		return err
	}

	return json.NewEncoder(w).Encode(eventInfo)
}

// title: event cancel
// path: /events/{uuid}/cancel
// method: POST
// produce: application/json
// responses:
//
//	204: OK
//	400: Invalid uuid or empty reason
//	401: Unauthorized
//	404: Not found
func eventCancel(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	uuid := r.URL.Query().Get(":uuid")
	if !bson.IsObjectIdHex(uuid) {
		msg := fmt.Sprintf("uuid parameter is not ObjectId: %s", uuid)
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	e, err := event.GetByHexID(ctx, uuid)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	reason := InputValue(r, "reason")
	if reason == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "reason is mandatory"}
	}
	scheme, err := permission.SafeGet(e.AllowedCancel.Scheme)
	if err != nil {
		return err
	}
	allowed := permission.Check(ctx, t, scheme, e.AllowedCancel.Contexts...)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err = e.TryCancel(ctx, reason, t.GetUserName())
	if err != nil {
		if err == event.ErrNotCancelable {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// title: event block list
// path: /events/blocks
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	204: No content
//	401: Unauthorized
func eventBlockList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	if !permission.Check(ctx, t, permission.PermEventBlockRead) {
		return permission.ErrUnauthorized
	}
	var active *bool
	if activeStr := InputValue(r, "active"); activeStr != "" {
		b, _ := strconv.ParseBool(activeStr)
		active = &b
	}
	blocks, err := event.ListBlocks(ctx, active)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(blocks)
}

// title: add event block
// path: /events/blocks
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	200: OK
//	400: Invalid data or empty reason
//	401: Unauthorized
func eventBlockAdd(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	if !permission.Check(ctx, t, permission.PermEventBlockAdd) {
		return permission.ErrUnauthorized
	}
	var block event.Block
	err = ParseInput(r, &block)
	if err != nil {
		return err
	}
	if block.Reason == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "reason is required"}
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeEventBlock},
		Kind:       permission.PermEventBlockAdd,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermEventBlockReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() {
		evt.Target.Value = block.ID.Hex()
		evt.Done(ctx, err)
	}()
	return event.AddBlock(ctx, &block)
}

// title: remove event block
// path: /events/blocks/{uuid}
// method: DELETE
// responses:
//
//	200: OK
//	400: Invalid uuid
//	401: Unauthorized
//	404: Active block with provided uuid not found
func eventBlockRemove(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	if !permission.Check(ctx, t, permission.PermEventBlockRemove) {
		return permission.ErrUnauthorized
	}
	uuid := r.URL.Query().Get(":uuid")
	if !bson.IsObjectIdHex(uuid) {
		msg := fmt.Sprintf("uuid parameter is not ObjectId: %s", uuid)
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	objID, err := primitive.ObjectIDFromHex(uuid)
	if err != nil {
		return err
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeEventBlock, Value: objID.Hex()},
		Kind:       permission.PermEventBlockRemove,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: []map[string]interface{}{
			{"name": "ID", "value": objID.Hex()},
		},
		Allowed: event.Allowed(permission.PermEventBlockReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	err = event.RemoveBlock(ctx, objID)
	if _, ok := err.(*event.ErrActiveEventBlockNotFound); ok {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return err
}

func suppressSensitiveEnvs(e *event.Event) error {
	if supressEnabled, _ := config.GetBool("events:suppress-sensitive-envs"); !supressEnabled {
		return nil
	}
	if e.Kind.Name != permission.PermAppDeploy.FullName() || len(e.StartCustomData.Value) == 0 {
		return nil
	}

	deployOptions := &app.DeployOptions{}

	err := e.StartCustomData.Unmarshal(deployOptions)
	if err != nil {
		return err
	}

	if deployOptions.App == nil {
		return nil
	}

	deployOptions.App.SuppressSensitiveEnvs()

	e.StartCustomData.Type, e.StartCustomData.Value, err = mongoBSON.MarshalValue(deployOptions)
	if err != nil {
		return err
	}
	return nil
}
