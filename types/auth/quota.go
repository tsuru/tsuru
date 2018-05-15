// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package auth

import (
	"errors"
	"fmt"
)

type Quota struct {
	Limit int `json:"limit"`
	InUse int `json:"inuse"`
}

// UnlimitedQuota is the struct which any new unlimited quota copies from.
var UnlimitedQuota = Quota{Limit: -1, InUse: 0}

type QuotaService interface {
	ReserveApp(email string) error
	ReleaseApp(email string) error
	ChangeLimit(email string, limit int) error
	FindByUserEmail(email string) (*Quota, error)
}

type QuotaStorage interface {
	IncInUse(email string, quantity int) error
	SetLimit(email string, limit int) error
	FindByUserEmail(email string) (*Quota, error)
}

type QuotaExceededError struct {
	Requested uint
	Available uint
}

func (err *QuotaExceededError) Error() string {
	return fmt.Sprintf("Quota exceeded. Available: %d, Requested: %d.", err.Available, err.Requested)
}

var (
	ErrCantRelease             = errors.New("Cannot release unreserved app")
	ErrLimitLowerThanAllocated = errors.New("New limit is lesser than the current allocated value")
)
