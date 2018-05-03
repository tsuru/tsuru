// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"errors"
	"net/http"
)

var (
	ErrWebHookAlreadyExists = errors.New("webhook already exists with the same name")
	ErrWebHookNotFound      = errors.New("webhook not found")
)

type WebHookEventFilter struct {
	TargetTypes  []string `json:"target_types" form:"target_types"`
	TargetValues []string `json:"target_values" form:"target_values"`
	KindTypes    []string `json:"kind_types" form:"kind_types"`
	KindNames    []string `json:"kind_names" form:"kind_names"`
	ErrorOnly    bool     `json:"error_only" form:"error_only"`
	SuccessOnly  bool     `json:"success_only" form:"success_only"`
}

type WebHook struct {
	Name        string             `json:"name" form:"name"`
	Description string             `json:"description" form:"description"`
	TeamOwner   string             `json:"team_owner" form:"team_owner"`
	EventFilter WebHookEventFilter `json:"event_filter" form:"event_filter"`
	URL         string             `json:"url" form:"url"`
	Headers     http.Header        `json:"headers" form:"headers"`
	Method      string             `json:"method" form:"method"`
	Body        string             `json:"body" form:"body"`
	Insecure    bool               `json:"insecure" form:"insecure"`
}

type WebHookService interface {
	Notify(evtID string)
	Create(WebHook) error
	Update(WebHook) error
	Delete(string) error
	Find(string) (WebHook, error)
	List([]string) ([]WebHook, error)
}

type WebHookStorage interface {
	Insert(WebHook) error
	Update(WebHook) error
	FindAllByTeams([]string) ([]WebHook, error)
	FindByName(string) (*WebHook, error)
	FindByEvent(f WebHookEventFilter, isSuccess bool) ([]WebHook, error)
	Delete(string) error
}
