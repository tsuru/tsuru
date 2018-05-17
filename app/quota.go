// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/quota"
)

type appQuotaService struct {
	storage quota.AppQuotaStorage
}

func QuotaService() (quota.AppQuotaService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &appQuotaService{dbDriver.AppQuotaStorage}, nil
}

// ReserveUnits implements ReserveUnits method from AppQuotaService interface
func (s *appQuotaService) ReserveUnits(appName string, quantity int) error {
	quota, err := s.storage.FindByAppName(appName)
	if err != nil {
		return err
	}
	err = s.CheckAppLimit(quota, quantity)
	if err != nil {
		return err
	}
	return s.storage.IncInUse(appName, quantity)
}

// CheckAppLimit implements CheckAppLimit method from AppQuotaService interface
func (s *appQuotaService) CheckAppLimit(q *quota.Quota, quantity int) error {
	if !q.IsUnlimited() && q.InUse+quantity > q.Limit {
		return &quota.QuotaExceededError{
			Available: uint(q.Limit - q.InUse),
			Requested: uint(quantity),
		}
	}
	return nil
}

// ReleaseUnits implements ReleaseUnits method from AppQuotaService interface
func (s *appQuotaService) ReleaseUnits(appName string, quantity int) error {
	quota, err := s.storage.FindByAppName(appName)
	if err != nil {
		return err
	}
	err = s.CheckAppUsage(quota, quantity)
	if err != nil {
		return err
	}
	return s.storage.IncInUse(appName, -1*quantity)
}

// CheckAppUsage implements CheckAppUsage method from AppQuotaService interface
func (s *appQuotaService) CheckAppUsage(q *quota.Quota, quantity int) error {
	if q.InUse-quantity < 0 {
		return quota.ErrNoReservedUnits
	}
	return nil
}

// ChangeLimit redefines the limit of the app. The new limit must be bigger
// than or equal to the current number of units in the app. The new limit may be
// smaller than 0, which means that the app should have an unlimited number of
// units.
// ChangeLimit implements ChangeLimit method from AppQuotaService interface
func (s *appQuotaService) ChangeLimit(appName string, limit int) error {
	q, err := s.storage.FindByAppName(appName)
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < q.InUse {
		return quota.ErrLimitLowerThanAllocated
	}
	return s.storage.SetLimit(appName, limit)
}

// ChangeInUse redefines the inuse units of the app. This new value must be smaller
// than or equal to the current limit of the app. It also must be a non negative number.
// ChangeInUse implements ChangeInUse method from AppQuotaService interface
func (s *appQuotaService) ChangeInUse(appName string, inUse int) error {
	q, err := s.storage.FindByAppName(appName)
	if err != nil {
		return err
	}
	if inUse < 0 {
		return quota.ErrLesserThanZero
	}
	if !q.IsUnlimited() && inUse > q.Limit {
		return &quota.QuotaExceededError{
			Requested: uint(inUse),
			Available: uint(q.Limit),
		}
	}
	return s.storage.SetInUse(appName, inUse)
}

// FindByAppName implements FindByAppName method from AppQuotaService interface
func (s *appQuotaService) FindByAppName(appName string) (*quota.Quota, error) {
	return s.storage.FindByAppName(appName)
}
