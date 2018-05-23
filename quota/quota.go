// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

import (
	"github.com/tsuru/tsuru/types/quota"
)

type QuotaService struct {
	Storage quota.QuotaStorage
}

// Inc implements Inc method from QuotaService interface
func (s *QuotaService) Inc(appName string, quantity int) error {
	quota, err := s.Storage.Get(appName)
	if err != nil {
		return err
	}
	err = s.checkLimit(quota, quantity)
	if err != nil {
		return err
	}
	return s.Storage.Inc(appName, quantity)
}

func (s *QuotaService) checkLimit(q *quota.Quota, quantity int) error {
	if !q.IsUnlimited() && q.InUse+quantity > q.Limit {
		return &quota.QuotaExceededError{
			Available: uint(q.Limit - q.InUse),
			Requested: uint(quantity),
		}
	}
	if q.InUse+quantity < 0 {
		return quota.ErrNotEnoughReserved
	}
	return nil
}

// SetLimit redefines the limit of the app. The new limit must be bigger
// than or equal to the current number of units in the app. The new limit may be
// smaller than 0, which means that the app should have an unlimited number of
// units.
// SetLimit implements SetLimit method from QuotaService interface
func (s *QuotaService) SetLimit(appName string, limit int) error {
	q, err := s.Storage.Get(appName)
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < q.InUse {
		return quota.ErrLimitLowerThanAllocated
	}
	return s.Storage.SetLimit(appName, limit)
}

// Set redefines the inuse units of the app. This new value must be smaller
// than or equal to the current limit of the app. It also must be a non negative number.
// Set implements Set method from QuotaService interface
func (s *QuotaService) Set(appName string, inUse int) error {
	q, err := s.Storage.Get(appName)
	if err != nil {
		return err
	}
	if inUse < 0 {
		return quota.ErrLessThanZero
	}
	if !q.IsUnlimited() && inUse > q.Limit {
		return &quota.QuotaExceededError{
			Requested: uint(inUse),
			Available: uint(q.Limit),
		}
	}
	return s.Storage.Set(appName, inUse)
}

// Get implements Get method from QuotaService interface
func (s *QuotaService) Get(appName string) (*quota.Quota, error) {
	return s.Storage.Get(appName)
}
