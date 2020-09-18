// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracker

import (
	"context"
	"time"
)

type TrackedInstance struct {
	Name       string
	Port       string
	TLSPort    string
	Addresses  []string
	LastUpdate time.Time
}

type InstanceService interface {
	LiveInstances(context.Context) ([]TrackedInstance, error)
	CurrentInstance(context.Context) (TrackedInstance, error)
}

type InstanceStorage interface {
	Notify(context.Context, TrackedInstance) error
	List(ctx context.Context, maxStale time.Duration) ([]TrackedInstance, error)
}
