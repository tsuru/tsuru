// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/tsuru/tsuru/permission"
)

var (
	ErrWebHookAlreadyExists = errors.New("webhook already exists with the same name")
	ErrWebHookNotFound      = errors.New("webhook not found")
)

type EventFilter struct {
	TargetTypes        []string
	TargetValues       []string
	KindTypes          []string
	KindNames          []string
	ErrorOnly          bool
	SuccessOnly        bool
	AllowedPermissions WebHookAllowedPermission
}

type WebHookAllowedPermission struct {
	Scheme   string
	Contexts []permission.PermissionContext
}

type WebHook struct {
	Name        string
	Description string
	TeamOwner   string
	EventFilter EventFilter
	URL         url.URL
	Headers     http.Header
	Method      string
	Body        string
	Insecure    bool
}

type WebHookService interface {
	Notify(evtID string)
}

type WebHookStorage interface {
	Insert(WebHook) error
	Update(WebHook) error
	FindAllByTeams([]string) ([]WebHook, error)
	FindByName(string) (*WebHook, error)
	FindByEvent(f EventFilter, isSuccess bool) ([]WebHook, error)
	Delete(string) error
}
