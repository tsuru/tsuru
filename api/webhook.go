// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

// title: webhook list
// path: /events/webhooks
// method: GET
// produce: application/json
// responses:
//   200: List webhooks
//   204: No content
func webhookList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctxs := permission.ContextsForPermission(t, permission.PermWebhookRead, permTypes.CtxTeam)
	var teams []string
	for _, c := range ctxs {
		if c.CtxType == permTypes.CtxGlobal {
			teams = nil
			break
		}
		teams = append(teams, c.Value)
	}
	webhooks, err := servicemanager.Webhook.List(teams)
	if err != nil {
		return err
	}
	if len(webhooks) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(webhooks)
}

// title: webhook info
// path: /events/webhooks/{name}
// method: GET
// produce: application/json
// responses:
//   200: Get webhook
//   404: Not found
//   401: Unauthorized
func webhookInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	webhookName := r.URL.Query().Get(":name")
	webhook, err := servicemanager.Webhook.Find(webhookName)
	if err != nil {
		if err == eventTypes.ErrWebhookNotFound {
			w.WriteHeader(http.StatusNotFound)
		}
		return err
	}
	ctx := permission.Context(permTypes.CtxTeam, webhook.TeamOwner)
	if !permission.Check(t, permission.PermWebhookRead, ctx) {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(webhook)
}

// title: webhook create
// path: /events/webhooks
// method: POST
// responses:
//   200: Webhook created
//   401: Unauthorized
//   400: Invalid webhook
//   409: Webhook already exists
func webhookCreate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	r.ParseForm()
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	var webhook eventTypes.Webhook
	err := dec.DecodeValues(&webhook, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: fmt.Sprintf("unable to parse webhook: %v", err)}
	}
	if webhook.TeamOwner == "" {
		webhook.TeamOwner, err = autoTeamOwner(t, permission.PermWebhookCreate)
		if err != nil {
			return err
		}
	}
	ctx := permission.Context(permTypes.CtxTeam, webhook.TeamOwner)
	if !permission.Check(t, permission.PermWebhookCreate, ctx) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeWebhook, Value: webhook.Name},
		Kind:       permission.PermWebhookCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermWebhookReadEvents, ctx),
	})
	if err != nil {
		return err
	}
	defer func() {
		evt.Done(err)
	}()
	err = servicemanager.Webhook.Create(webhook)
	if err == eventTypes.ErrWebhookAlreadyExists {
		w.WriteHeader(http.StatusConflict)
	}
	return err
}

// title: webhook update
// path: /events/webhooks/{name}
// method: PUT
// responses:
//   200: Webhook updated
//   401: Unauthorized
//   400: Invalid webhook
//   404: Webhook not found
func webhookUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	r.ParseForm()
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	var webhook eventTypes.Webhook
	err := dec.DecodeValues(&webhook, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: fmt.Sprintf("unable to parse webhook: %v", err)}
	}
	webhook.Name = r.URL.Query().Get(":name")
	ctx := permission.Context(permTypes.CtxTeam, webhook.TeamOwner)
	if !permission.Check(t, permission.PermWebhookUpdate, ctx) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeWebhook, Value: webhook.Name},
		Kind:       permission.PermWebhookUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermWebhookReadEvents, ctx),
	})
	if err != nil {
		return err
	}
	defer func() {
		evt.Done(err)
	}()
	err = servicemanager.Webhook.Update(webhook)
	if err == eventTypes.ErrWebhookNotFound {
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

// title: webhook delete
// path: /events/webhooks/{name}
// method: DELETE
// responses:
//   200: Webhook deleted
//   401: Unauthorized
//   404: Webhook not found
func webhookDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	webhookName := r.URL.Query().Get(":name")
	webhook, err := servicemanager.Webhook.Find(webhookName)
	if err != nil {
		if err == eventTypes.ErrWebhookNotFound {
			w.WriteHeader(http.StatusNotFound)
		}
		return err
	}
	ctx := permission.Context(permTypes.CtxTeam, webhook.TeamOwner)
	if !permission.Check(t, permission.PermWebhookDelete, ctx) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeWebhook, Value: webhook.Name},
		Kind:       permission.PermWebhookDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermWebhookReadEvents, ctx),
	})
	if err != nil {
		return err
	}
	defer func() {
		evt.Done(err)
	}()
	return servicemanager.Webhook.Delete(webhookName)
}
