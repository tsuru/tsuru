// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"net/url"

	"github.com/tsuru/tsuru/types/app/image"
	"github.com/tsuru/tsuru/types/bind"
	"github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/types/router"
	"github.com/tsuru/tsuru/types/volume"
)

// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.
type App struct {
	Env             map[string]bind.EnvVar
	ServiceEnvs     []bind.ServiceEnvVar
	Platform        string `bson:"framework"`
	PlatformVersion string
	Name            string
	CName           []string
	CertIssuers     CertIssuers
	Teams           []string
	TeamOwner       string
	Owner           string
	Plan            Plan
	UpdatePlatform  bool
	Pool            string
	Description     string
	Router          string // TODO: drop Router field and use just on inputApp
	RouterOpts      map[string]string
	Deploys         uint
	Tags            []string
	Error           string
	Routers         []AppRouter
	Metadata        Metadata
	Processes       []Process

	// UUID is a v4 UUID lazily generated on the first call to GetUUID()
	UUID string

	Quota quota.Quota
}

var CertIssuerDotReplacement = "_dot_"

type CertIssuers map[string]string

const (
	DefaultAppDir = "/home/application/current"
)

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
	Tags        []string
	Extra       map[string][]string
}

type AppService interface {
	GetByName(ctx context.Context, name string) (*App, error)
	List(ctx context.Context, filter *Filter) ([]*App, error)
	GetHealthcheckData(ctx context.Context, app *App) (router.HealthcheckData, error)
	GetAddresses(ctx context.Context, app *App) ([]string, error)
	GetInternalBindableAddresses(ctx context.Context, app *App) ([]string, error)
	GetRegistry(ctx context.Context, app *App) (image.ImageRegistry, error)

	AddInstance(ctx context.Context, app *App, addArgs bind.AddInstanceArgs) error
	RemoveInstance(ctx context.Context, app *App, removeArgs bind.RemoveInstanceArgs) error
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
	Tags        []string `json:"tags"`
	Metadata    Metadata `json:"metadata"`

	Units                   []provision.Unit                 `json:"units"`
	InternalAddresses       []AppInternalAddress             `json:"internalAddresses,omitempty"`
	Autoscale               []provision.AutoScaleSpec        `json:"autoscale,omitempty"`
	UnitsMetrics            []provision.UnitMetric           `json:"unitsMetrics,omitempty"`
	AutoscaleRecommendation []provision.RecommendedResources `json:"autoscaleRecommendation,omitempty"`

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

	DashboardURL string `json:"dashboardURL,omitempty"`
}

type AppInternalAddress struct {
	Domain     string
	Protocol   string
	Port       int32
	TargetPort int `json:"TargetPort,omitempty"`
	Version    string
	Process    string
}

// AppResume is a minimal representation of the app, created to make appList
// faster and transmit less data.
type AppResume struct {
	Name        string           `json:"name"`
	Pool        string           `json:"pool"`
	TeamOwner   string           `json:"teamowner"`
	Plan        Plan             `json:"plan"`
	Units       []provision.Unit `json:"units"`
	CName       []string         `json:"cname"`
	IP          string           `json:"ip"`
	Routers     []AppRouter      `json:"routers"`
	Tags        []string         `json:"tags"`
	Error       string           `json:"error,omitempty"`
	Platform    string           `json:"platform,omitempty"`
	Description string           `json:"description,omitempty"`
	Metadata    Metadata         `json:"metadata,omitempty"`
}
