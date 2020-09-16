// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

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

type QuotaItem interface {
	GetName() string
}

type QuotaItemInUse interface {
	QuotaItem
	GetQuotaInUse() (int, error)
}

type QuotaService interface {
	Inc(item QuotaItem, delta int) error
	Set(item QuotaItem, quantity int) error
	SetLimit(item QuotaItem, limit int) error
	Get(item QuotaItem) (*Quota, error)
}

type QuotaStorage interface {
	SetLimit(name string, limit int) error
	Get(name string) (*Quota, error)
	Set(name string, quantity int) error
}

type QuotaExceededError struct {
	Requested uint
	Available uint
}

func (err *QuotaExceededError) Error() string {
	return fmt.Sprintf("Quota exceeded. Available: %d, Requested: %d.", err.Available, err.Requested)
}

var (
	ErrNotEnoughReserved       = errors.New("Not enough reserved items")
	ErrLimitLowerThanAllocated = errors.New("New limit is less than the current allocated value")
	ErrLessThanZero            = errors.New("Invalid value, cannot be less than 0")
	ErrQuotaNotFound           = errors.New("quota not found")
)
