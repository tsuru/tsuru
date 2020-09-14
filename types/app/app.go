// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"net/url"
)

type App interface {
	GetName() string
	GetPool() string
	GetTeamOwner() string
	GetTeamsName() []string
	GetPlatform() string
	GetPlatformVersion() string
	GetDeploys() uint
	GetUpdatePlatform() bool
}

type AppRouter struct {
	Name         string            `json:"name"`
	Opts         map[string]string `json:"opts"`
	Address      string            `json:"address" bson:"-"`
	Addresses    []string          `json:"addresses" bson:"-"`
	Type         string            `json:"type" bson:"-"`
	Status       string            `json:"status,omitempty" bson:"-"`
	StatusDetail string            `json:"status-detail,omitempty" bson:"-"`
}

type RoutableAddresses struct {
	Prefix    string
	Addresses []*url.URL
	ExtraData map[string]string
}

type AppService interface {
	GetByName(ctx context.Context, name string) (App, error)
}
