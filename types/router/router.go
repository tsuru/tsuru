// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
)

var ErrDynamicRouterNotFound = errors.New("dynamic router not found")

type DynamicRouter struct {
	Name           string
	Type           string
	ReadinessGates []string
	Config         map[string]interface{}
}

type PlanRouter struct {
	Name           string                 `json:"name"`
	Type           string                 `json:"type"`
	Info           map[string]string      `json:"info"`
	Config         map[string]interface{} `json:"config"`
	ReadinessGates []string               `json:"readinessGates"`
	Dynamic        bool                   `json:"dynamic"`
	Default        bool                   `json:"default"`
}

func (r *DynamicRouter) ToPlanRouter() PlanRouter {
	return PlanRouter{
		Name:           r.Name,
		Type:           r.Type,
		Config:         r.Config,
		ReadinessGates: r.ReadinessGates,
		Dynamic:        true,
	}
}

type DynamicRouterService interface {
	Get(ctx context.Context, name string) (*DynamicRouter, error)
	List(context.Context) ([]DynamicRouter, error)
	Remove(ctx context.Context, name string) error
	Create(context.Context, DynamicRouter) error
	Update(context.Context, DynamicRouter) error
}

type DynamicRouterStorage interface {
	Save(context.Context, DynamicRouter) error
	Get(ctx context.Context, name string) (*DynamicRouter, error)
	List(context.Context) ([]DynamicRouter, error)
	Remove(ctx context.Context, name string) error
}

type HealthcheckData struct {
	Path    string
	TCPOnly bool
}

func (hc *HealthcheckData) String() string {
	if hc.TCPOnly {
		return "tcp only"
	}
	path := hc.Path
	if path == "" {
		path = "/"
	}
	return fmt.Sprintf("path: %q", path)
}
