// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"sync"

	"github.com/pkg/errors"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type appQuotaService struct {
	storage appTypes.AppQuotaStorage
	mutex   *sync.Mutex
}

func (s *appQuotaService) ReserveUnits(quota *appTypes.AppQuota, quantity int) error {
	err := s.CheckAppLimit(quota, quantity)
	if err != nil {
		return err
	}
	err = s.storage.IncInUse(s, quota, quantity)
	if err != nil {
		return err
	}
	quota.Limit += 1
	return nil
}

func (s *appQuotaService) CheckAppLimit(quota *appTypes.AppQuota, quantity int) error {
	if !quota.Unlimited() && quota.InUse+quantity > quota.Limit {
		return &appTypes.AppQuotaExceededError{
			Available: uint(quota.Limit - quota.InUse),
			Requested: uint(quantity),
		}
	}
	return nil
}

func (s *appQuotaService) ReleaseUnits(quota *appTypes.AppQuota, quantity int) error {
	s.mutex.Lock()
	err := s.CheckAppUsage(quota, quantity)
	quota.InUse -= quantity
	s.mutex.Unlock()
	if err != nil {
		s.mutex.Lock()
		quota.InUse += quantity
		s.mutex.Unlock()
		return err
	}
	err = s.storage.IncInUse(s, quota, -1*quantity)
	if err != nil {
		s.mutex.Lock()
		quota.InUse += quantity
		s.mutex.Unlock()
		return err
	}
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
func (s *appQuotaService) ChangeLimitQuota(quota *appTypes.AppQuota, limit int) error {
	if limit < 0 {
		limit = -1
	} else if limit < quota.InUse {
		return appTypes.ErrLimitLowerThanAllocated
	}
	quota.Limit = limit
	err := s.storage.SetLimit(quota.AppName, quota.Limit)
	if err != nil {
		return err
	}
	return nil
}

func (s *appQuotaService) ChangeInUseQuota(quota *appTypes.AppQuota, inUse int) error {
	if inUse < 0 {
		return errors.New("invalid value, cannot be lesser than 0")
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
