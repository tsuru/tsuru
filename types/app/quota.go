// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
)

type AppQuota struct {
	AppName string `json:"appname"`
	Limit   int    `json:"limit"`
	InUse   int    `json:"inuse"`
}

func (q *AppQuota) Unlimited() bool {
	return -1 == q.Limit
}

type AppQuotaService interface {
	CheckAppUsage(quota *AppQuota, quantity int) error
	CheckAppLimit(quota *AppQuota, quantity int) error
	ReserveUnits(quota *AppQuota, quantity int) error
	ReleaseUnits(quota *AppQuota, quantity int) error
	ChangeLimitQuota(quota *AppQuota, limit int) error
	ChangeInUseQuota(quota *AppQuota, inUse int) error
}

type AppQuotaStorage interface {
	IncInUse(service AppQuotaService, quota *AppQuota, quantity int) error
	SetLimit(appName string, limit int) error
	SetInUse(appName string, inUse int) error
}

type AppQuotaExceededError struct {
	Requested uint
	Available uint
}

func (err *AppQuotaExceededError) Error() string {
	return fmt.Sprintf("Quota exceeded. Available: %d, Requested: %d.", err.Available, err.Requested)
}

var (
	ErrNoReservedUnits         = errors.New("Not enough reserved units")
	ErrLimitLowerThanAllocated = errors.New("new limit is lesser than the current allocated value")
)
