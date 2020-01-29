// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"errors"
	"net/http"
)

var (
	ErrWebhookAlreadyExists = errors.New("webhook already exists with the same name")
	ErrWebhookNotFound      = errors.New("webhook not found")
	ErrWebhookLoop          = errors.New("webhook can't trigger another webhook")
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
	Notify(evtID string)
	Create(Webhook) error
	Update(Webhook) error
	Delete(string) error
	Find(string) (Webhook, error)
	List([]string) ([]Webhook, error)
}

type WebhookStorage interface {
	Insert(Webhook) error
	Update(Webhook) error
	FindAllByTeams([]string) ([]Webhook, error)
	FindByName(string) (*Webhook, error)
	FindByEvent(f WebhookEventFilter, isSuccess bool) ([]Webhook, error)
	Delete(string) error
}
