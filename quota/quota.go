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
func (s *QuotaService) Inc(item quota.QuotaItem, quantity int) error {
	quota, err := s.Storage.Get(item.GetName())
	if err != nil {
		return err
	}
	err = s.fixInUse(item, quota)
	if err != nil {
		return err
	}
	err = s.checkLimit(quota, quantity)
	if err != nil {
		return err
	}
	return s.Storage.Set(item.GetName(), quota.InUse+quantity)
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

// SetLimit redefines the limit of the named resource. The new limit must be
// bigger than or equal to the current isUse. The new
// limit may be smaller than 0, which means that the app should have an
// unlimited number of units. SetLimit implements SetLimit method from
// QuotaService interface
func (s *QuotaService) SetLimit(item quota.QuotaItem, limit int) error {
	q, err := s.Storage.Get(item.GetName())
	if err != nil {
		return err
	}
	err = s.fixInUse(item, q)
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < q.InUse {
		return quota.ErrLimitLowerThanAllocated
	}
	return s.Storage.SetLimit(item.GetName(), limit)
}

// Set redefines the inuse value for the named resource. This new value must be
// smaller than or equal to the current limit. It also must be a non negative
// number.
func (s *QuotaService) Set(item quota.QuotaItem, inUse int) error {
	q, err := s.Storage.Get(item.GetName())
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
	return s.Storage.Set(item.GetName(), inUse)
}

func (s *QuotaService) Get(item quota.QuotaItem) (*quota.Quota, error) {
	q, err := s.Storage.Get(item.GetName())
	if err != nil {
		return nil, err
	}
	err = s.fixInUse(item, q)
	if err != nil {
		return nil, err
	}
	return q, nil
}

func (s *QuotaService) fixInUse(item quota.QuotaItem, q *quota.Quota) error {
	var err error
	if inuse, ok := item.(quota.QuotaItemInUse); ok {
		q.InUse, err = inuse.GetQuotaInUse()
	}
	return err
}
