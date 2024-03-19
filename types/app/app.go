// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"net/url"

	"github.com/tsuru/tsuru/types/app/image"
	"github.com/tsuru/tsuru/types/bind"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/types/volume"
)

type App interface {
	GetName() string
	GetPool() string
	GetTeamOwner() string
	GetTeamsName() []string
	GetPlatform() string
	GetPlatformVersion() string
	GetRegistry() (image.ImageRegistry, error)
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

type Filter struct {
	Name        string
	NameMatches string
	Platform    string
	TeamOwner   string
	UserOwner   string
	Pool        string
	Pools       []string
	Statuses    []string
	Locked      bool
	Tags        []string
	Extra       map[string][]string
}

type AppService interface {
	GetByName(ctx context.Context, name string) (App, error)
	List(ctx context.Context, filter *Filter) ([]App, error)
}

type AppInfo struct {
	Name        string   `json:"name"`
	Platform    string   `json:"platform"`
	Teams       []string `json:"teams"`
	Plan        *Plan    `json:"plan"`
	CName       []string `json:"cname"`
	Owner       string   `json:"owner"` // we may move this to createdBy
	Pool        string   `json:"pool"`
	Description string   `json:"description"`
	Deploys     uint     `json:"deploys"`
	TeamOwner   string   `json:"teamowner"`
	Lock        AppLock  `json:"lock"`
	Tags        []string `json:"tags"`
	Metadata    Metadata `json:"metadata"`

	Units                   any `json:"units,omitempty"`                   // TODO: convert to typed field
	InternalAddresses       any `json:"internalAddresses,omitempty"`       // TODO: convert to typed field
	Autoscale               any `json:"autoscale,omitempty"`               // TODO: convert to typed field
	UnitsMetrics            any `json:"unitsMetrics,omitempty"`            // TODO: convert to typed field
	AutoscaleRecommendation any `json:"autoscaleRecommendation,omitempty"` // TODO: convert to typed field or deprecate

	Provisioner          string                     `json:"provisioner,omitempty"`
	Cluster              string                     `json:"cluster,omitempty"`
	Processes            []Process                  `json:"processes,omitempty"`
	Routers              []AppRouter                `json:"routers"`
	VolumeBinds          []volume.VolumeBind        `json:"volumeBinds,omitempty"`
	ServiceInstanceBinds []bind.ServiceInstanceBind `json:"serviceInstanceBinds"`

	IP         string            `json:"ip,omitempty"`
	Router     string            `json:"router,omitempty"`
	RouterOpts map[string]string `json:"routeropts"`
	Quota      *quota.Quota      `json:"quota,omitempty"`
	Error      string            `json:"error,omitempty"`
}
