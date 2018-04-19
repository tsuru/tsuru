// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	appTypes "github.com/tsuru/tsuru/types/app"
)

type appQuotaService struct {
	storage appTypes.AppQuotaStorage
}

func (s *appQuotaService) checkAppExists(appName string) error {
	_, err := s.storage.FindByAppName(appName)
	if err != nil {
		if err == appTypes.ErrAppNotFound {
			return ErrAppNotFound
		}
		return err
	}
	return nil
}

func (s *appQuotaService) ReserveUnits(quota *appTypes.AppQuota, quantity int) error {
	err := s.checkAppExists(quota.AppName)
	if err != nil {
		return err
	}
	err = s.CheckAppLimit(quota, quantity)
	if err != nil {
		return err
	}
	err = s.storage.IncInUse(quota, quantity)
	if err != nil {
		return err
	}
	quota.InUse += quantity
	return nil
}

func (s *appQuotaService) CheckAppLimit(quota *appTypes.AppQuota, quantity int) error {
	err := s.checkAppExists(quota.AppName)
	if err != nil {
		return err
	}
	if !quota.Unlimited() && quota.InUse+quantity > quota.Limit {
		return &appTypes.AppQuotaExceededError{
			Available: uint(quota.Limit - quota.InUse),
			Requested: uint(quantity),
		}
	}
	return nil
}

func (s *appQuotaService) ReleaseUnits(quota *appTypes.AppQuota, quantity int) error {
	err := s.checkAppExists(quota.AppName)
	if err != nil {
		return err
	}
	err = s.CheckAppUsage(quota, quantity)
	if err != nil {
		return err
	}
	err = s.storage.IncInUse(quota, -1*quantity)
	if err != nil {
		return err
	}
	quota.InUse -= quantity
	return nil
}

func (s *appQuotaService) CheckAppUsage(quota *appTypes.AppQuota, quantity int) error {
	if quota.InUse-quantity < 0 {
		return appTypes.ErrNoReservedUnits
	}
	return nil
}

// ChangeQuota redefines the limit of the app. The new limit must be bigger
// than or equal to the current number of units in the app. The new limit may be
// smaller than 0, which means that the app should have an unlimited number of
// units.
func (s *appQuotaService) ChangeLimit(quota *appTypes.AppQuota, limit int) error {
	err := s.checkAppExists(quota.AppName)
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < quota.InUse {
		return appTypes.ErrLimitLowerThanAllocated
	}
	err = s.storage.SetLimit(quota.AppName, quota.Limit)
	if err != nil {
		return err
	}
	quota.Limit = limit
	return nil
}

func (s *appQuotaService) ChangeInUse(quota *appTypes.AppQuota, inUse int) error {
	err := s.checkAppExists(quota.AppName)
	if err != nil {
		return err
	}
	if inUse < 0 {
		return appTypes.ErrLesserThanZero
	}
	if !quota.Unlimited() && inUse > quota.Limit {
		return &appTypes.AppQuotaExceededError{
			Requested: uint(inUse),
			Available: uint(quota.Limit),
		}
	}
	s.storage.SetInUse(quota.AppName, inUse)
	quota.InUse = inUse
	return nil
}

func (s *appQuotaService) FindByAppName(appName string) (*appTypes.AppQuota, error) {
	return s.storage.FindByAppName(appName)
}
