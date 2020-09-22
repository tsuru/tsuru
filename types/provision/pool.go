// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"context"
	"errors"
)

var (
	ErrPoolNotFound      = errors.New("pool does not exist")
	ErrTooManyPoolsFound = errors.New("too many pools found")
)

type Pool struct {
	Name        string `bson:"_id"`
	Provisioner string
	Default     bool
}

type PoolStorage interface {
	FindAll(ctx context.Context) ([]Pool, error)
	FindByName(ctx context.Context, name string) (*Pool, error)
}

type PoolService interface {
	List(ctx context.Context) ([]Pool, error)
	FindByName(ctx context.Context, name string) (*Pool, error)
}
