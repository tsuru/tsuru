// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
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
//
//	200: List webhooks
//	204: No content
func webhookList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	ctxs := permission.ContextsForPermission(t, permission.PermWebhookRead, permTypes.CtxTeam)
	var teams []string
	for _, c := range ctxs {
		if c.CtxType == permTypes.CtxGlobal {
			teams = nil
			break
		}
		teams = append(teams, c.Value)
	}
	webhooks, err := servicemanager.Webhook.List(ctx, teams)
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
//
//	200: Get webhook
//	404: Not found
//	401: Unauthorized
func webhookInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	webhookName := r.URL.Query().Get(":name")
	webhook, err := servicemanager.Webhook.Find(ctx, webhookName)
	if err != nil {
		if err == eventTypes.ErrWebhookNotFound {
			w.WriteHeader(http.StatusNotFound)
		}
		return err
	}
	permissionCtx := permission.Context(permTypes.CtxTeam, webhook.TeamOwner)
	if !permission.Check(t, permission.PermWebhookRead, permissionCtx) {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(webhook)
}

// title: webhook create
// path: /events/webhooks
// method: POST
// responses:
//
//	200: Webhook created
//	401: Unauthorized
//	400: Invalid webhook
//	409: Webhook already exists
func webhookCreate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	var webhook eventTypes.Webhook
	err := ParseInput(r, &webhook)
	if err != nil {
		return err
	}
	if webhook.TeamOwner == "" {
		webhook.TeamOwner, err = autoTeamOwner(ctx, t, permission.PermWebhookCreate)
		if err != nil {
			return err
		}
	}
	permCtx := permission.Context(permTypes.CtxTeam, webhook.TeamOwner)
	if !permission.Check(t, permission.PermWebhookCreate, permCtx) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     event.Target{Type: event.TargetTypeWebhook, Value: webhook.Name},
		Kind:       permission.PermWebhookCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermWebhookReadEvents, permCtx),
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
//
//	200: Webhook updated
//	401: Unauthorized
//	400: Invalid webhook
//	404: Webhook not found
func webhookUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	var webhook eventTypes.Webhook
	err := ParseInput(r, &webhook)
	if err != nil {
		return err
	}
	webhook.Name = r.URL.Query().Get(":name")
	permissionCtx := permission.Context(permTypes.CtxTeam, webhook.TeamOwner)
	if !permission.Check(t, permission.PermWebhookUpdate, permissionCtx) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     event.Target{Type: event.TargetTypeWebhook, Value: webhook.Name},
		Kind:       permission.PermWebhookUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermWebhookReadEvents, permissionCtx),
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
//
//	200: Webhook deleted
//	401: Unauthorized
//	404: Webhook not found
func webhookDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	webhookName := r.URL.Query().Get(":name")
	webhook, err := servicemanager.Webhook.Find(ctx, webhookName)
	if err != nil {
		if err == eventTypes.ErrWebhookNotFound {
			w.WriteHeader(http.StatusNotFound)
		}
		return err
	}
	permissionCtx := permission.Context(permTypes.CtxTeam, webhook.TeamOwner)
	if !permission.Check(t, permission.PermWebhookDelete, permissionCtx) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     event.Target{Type: event.TargetTypeWebhook, Value: webhook.Name},
		Kind:       permission.PermWebhookDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermWebhookReadEvents, permissionCtx),
	})
	if err != nil {
		return err
	}
	defer func() {
		evt.Done(err)
	}()
	return servicemanager.Webhook.Delete(webhookName)
}
