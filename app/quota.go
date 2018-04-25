// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	appTypes "github.com/tsuru/tsuru/types/app"
)

type appQuotaService struct {
	storage appTypes.QuotaStorage
}

// ReserveUnits implements ReserveUnits method from AppQuotaService interface
func (s *appQuotaService) ReserveUnits(appName string, quantity int) error {
	quota, err := s.storage.FindByAppName(appName)
	if err != nil {
		if err == appTypes.ErrAppNotFound {
			return ErrAppNotFound
		}
		return err
	}
	err = s.CheckAppLimit(quota, quantity)
	if err != nil {
		return err
	}
	return s.storage.IncInUse(appName, quantity)
}

// CheckAppLimit implements CheckAppLimit method from AppQuotaService interface
func (s *appQuotaService) CheckAppLimit(quota *appTypes.Quota, quantity int) error {
	if !quota.Unlimited() && quota.InUse+quantity > quota.Limit {
		return &appTypes.QuotaExceededError{
			Available: uint(quota.Limit - quota.InUse),
			Requested: uint(quantity),
		}
	}
	return nil
}

// ReleaseUnits implements ReleaseUnits method from AppQuotaService interface
func (s *appQuotaService) ReleaseUnits(appName string, quantity int) error {
	quota, err := s.storage.FindByAppName(appName)
	if err != nil {
		if err == appTypes.ErrAppNotFound {
			return ErrAppNotFound
		}
		return err
	}
	err = s.CheckAppUsage(quota, quantity)
	if err != nil {
		return err
	}
	return s.storage.IncInUse(appName, -1*quantity)
}

// CheckAppUsage implements CheckAppUsage method from AppQuotaService interface
func (s *appQuotaService) CheckAppUsage(quota *appTypes.Quota, quantity int) error {
	if quota.InUse-quantity < 0 {
		return appTypes.ErrNoReservedUnits
	}
	return nil
}

// ChangeLimit redefines the limit of the app. The new limit must be bigger
// than or equal to the current number of units in the app. The new limit may be
// smaller than 0, which means that the app should have an unlimited number of
// units.
// ChangeLimit implements ChangeLimit method from AppQuotaService interface
func (s *appQuotaService) ChangeLimit(appName string, limit int) error {
	quota, err := s.storage.FindByAppName(appName)
	if err != nil {
		if err == appTypes.ErrAppNotFound {
			return ErrAppNotFound
		}
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < quota.InUse {
		return appTypes.ErrLimitLowerThanAllocated
	}
	return s.storage.SetLimit(appName, limit)
}

// ChangeInUse redefines the inuse units of the app. This new value must be smaller
// than or equal to the current limit of the app. It also must be a non negative number.
// ChangeInUse implements ChangeInUse method from AppQuotaService interface
func (s *appQuotaService) ChangeInUse(appName string, inUse int) error {
	quota, err := s.storage.FindByAppName(appName)
	if err != nil {
		if err == appTypes.ErrAppNotFound {
			return ErrAppNotFound
		}
		return err
	}
	if inUse < 0 {
		return appTypes.ErrLesserThanZero
	}
	if !quota.Unlimited() && inUse > quota.Limit {
		return &appTypes.QuotaExceededError{
			Requested: uint(inUse),
			Available: uint(quota.Limit),
		}
	}
	return s.storage.SetInUse(appName, inUse)
}

// FindByAppName implements FindByAppName method from AppQuotaService interface
func (s *appQuotaService) FindByAppName(appName string) (*appTypes.Quota, error) {
	return s.storage.FindByAppName(appName)
}
