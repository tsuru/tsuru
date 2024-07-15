// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"context"
	"errors"
	"net/http"
)

var (
	ErrWebhookAlreadyExists = errors.New("webhook already exists with the same name")
	ErrWebhookNotFound      = errors.New("webhook not found")
)

type WebhookEventFilter struct {
	TargetTypes  []string `json:"target_types" form:"target_types"`
	TargetValues []string `json:"target_values" form:"target_values"`
	KindTypes    []string `json:"kind_types" form:"kind_types"`
	KindNames    []string `json:"kind_names" form:"kind_names"`
	ErrorOnly    bool     `json:"error_only" form:"error_only"`
	SuccessOnly  bool     `json:"success_only" form:"success_only"`
}

type Webhook struct {
	Name        string             `json:"name" form:"name"`
	Description string             `json:"description" form:"description"`
	TeamOwner   string             `json:"team_owner" form:"team_owner"`
	EventFilter WebhookEventFilter `json:"event_filter" form:"event_filter"`
	URL         string             `json:"url" form:"url"`
	ProxyURL    string             `json:"proxy_url" form:"proxy_url"`
	Headers     http.Header        `json:"headers" form:"headers"`
	Method      string             `json:"method" form:"method"`
	Body        string             `json:"body" form:"body"`
	Insecure    bool               `json:"insecure" form:"insecure"`
}

type WebhookService interface {
	Notify(ctx context.Context, evtID string)
	Create(context.Context, Webhook) error
	Update(context.Context, Webhook) error
	Delete(context.Context, string) error
	Find(context.Context, string) (Webhook, error)
	List(context.Context, []string) ([]Webhook, error)
}

type WebhookStorage interface {
	Insert(context.Context, Webhook) error
	Update(context.Context, Webhook) error
	FindAllByTeams(context.Context, []string) ([]Webhook, error)
	FindByName(context.Context, string) (*Webhook, error)
	FindByEvent(ctx context.Context, f WebhookEventFilter, isSuccess bool) ([]Webhook, error)
	Delete(context.Context, string) error
}
