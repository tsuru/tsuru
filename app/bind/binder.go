// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bind provides interfaces and types for use when binding an app to a
// service.
package bind

import (
	"context"

	bindTypes "github.com/tsuru/tsuru/types/bind"
)

type App interface {
	// GetAddresses returns the app addresses.
	GetAddresses(ctx context.Context) ([]string, error)

	// GetInternalAddresses returns the app bindable addresses inside the cluster, if any.
	GetInternalBindableAddresses(ctx context.Context) ([]string, error)

	// GetName returns the app name.
	GetName() string

	// GetUUID returns the App v4 UUID
	GetUUID(ctx context.Context) (string, error)

	// AddInstance adds an instance to the application.
	AddInstance(ctx context.Context, args bindTypes.AddInstanceArgs) error

	// RemoveInstance removes an instance from the application.
	RemoveInstance(ctx context.Context, args bindTypes.RemoveInstanceArgs) error
}
