// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

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

func (q *Quota) IsUnlimited() bool {
	return -1 == q.Limit
}

type QuotaService interface {
	CheckAppUsage(quota *Quota, quantity int) error
	CheckAppLimit(quota *Quota, quantity int) error
	ReserveUnits(appName string, quantity int) error
	ReleaseUnits(appName string, quantity int) error
	ChangeLimit(appName string, limit int) error
	ChangeInUse(appName string, inUse int) error
	FindByAppName(appName string) (*Quota, error)
}

type QuotaStorage interface {
	IncInUse(appName string, quantity int) error
	SetLimit(appName string, limit int) error
	SetInUse(appName string, inUse int) error
	FindByAppName(appName string) (*Quota, error)
}

type QuotaExceededError struct {
	Requested uint
	Available uint
}

func (err *QuotaExceededError) Error() string {
	return fmt.Sprintf("Quota exceeded. Available: %d, Requested: %d.", err.Available, err.Requested)
}

var (
	ErrNoReservedUnits         = errors.New("Not enough reserved units")
	ErrLimitLowerThanAllocated = errors.New("new limit is lesser than the current allocated value")
	ErrLesserThanZero          = errors.New("invalid value, cannot be lesser than 0")
	ErrAppNotFound             = errors.New("App not found")
)
