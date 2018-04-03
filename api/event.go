// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ajg/form"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
)

// title: event list
// path: /events
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func eventList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: fmt.Sprintf("unable to parse event filters: %s", err)}
	}
	var filter *event.Filter
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	err = dec.DecodeValues(&filter, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: fmt.Sprintf("unable to parse event filters: %s", err)}
	}
	filter.LoadKindNames(r.Form)
	filter.PruneUserValues()
	filter.Permissions, err = t.Permissions()
	if err != nil {
		return err
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
	scheme, err := permission.SafeGet(e.Allowed.Scheme)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, scheme, e.Allowed.Contexts...)
	if !allowed {
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
	scheme, err := permission.SafeGet(e.AllowedCancel.Scheme)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, scheme, e.AllowedCancel.Contexts...)
	if !allowed {
		return permission.ErrUnauthorized
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

// title: event block list
// path: /events/blocks
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
//   401: Unauthorized
func eventBlockList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermEventBlockRead) {
		return permission.ErrUnauthorized
	}
	err := r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var active *bool
	if activeStr := r.FormValue("active"); activeStr != "" {
		b, _ := strconv.ParseBool(activeStr)
		active = &b
	}
	blocks, err := event.ListBlocks(active)
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
//   200: OK
//   400: Invalid data or empty reason
//   401: Unauthorized
func eventBlockAdd(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermEventBlockAdd) {
		return permission.ErrUnauthorized
	}
	r.ParseForm()
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	var block event.Block
	err = dec.DecodeValues(&block, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: fmt.Sprintf("unable to parse block: %s", err)}
	}
	if block.Reason == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "reason is required"}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeEventBlock},
		Kind:       permission.PermEventBlockAdd,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermEventBlockReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() {
		evt.Target.Value = block.ID.Hex()
		evt.Done(err)
	}()
	return event.AddBlock(&block)
}

// title: remove event block
// path: /events/blocks/{uuid}
// method: DELETE
// responses:
//   200: OK
//   400: Invalid uuid
//   401: Unauthorized
//   404: Active block with provided uuid not found
func eventBlockRemove(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermEventBlockRemove) {
		return permission.ErrUnauthorized
	}
	uuid := r.URL.Query().Get(":uuid")
	if !bson.IsObjectIdHex(uuid) {
		msg := fmt.Sprintf("uuid parameter is not ObjectId: %s", uuid)
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	objID := bson.ObjectIdHex(uuid)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeEventBlock, Value: objID.Hex()},
		Kind:   permission.PermEventBlockRemove,
		Owner:  t,
		CustomData: []map[string]interface{}{
			{"name": "ID", "value": objID.Hex()},
		},
		Allowed: event.Allowed(permission.PermEventBlockReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = event.RemoveBlock(objID)
	if _, ok := err.(*event.ErrActiveEventBlockNotFound); ok {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return err
}
