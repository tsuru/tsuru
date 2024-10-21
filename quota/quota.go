// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

import (
	"context"

	"github.com/tsuru/tsuru/types/quota"
)

type QuotaService[I quota.QuotaItem] struct {
	Storage quota.QuotaStorage
}

// Inc implements Inc method from QuotaService interface
func (s *QuotaService[I]) Inc(ctx context.Context, item I, quantity int) error {
	quota, err := s.Storage.Get(ctx, item.GetName())
	if err != nil {
		return err
	}
	err = s.checkLimit(quota, quantity)
	if err != nil {
		return err
	}
	return s.Storage.Set(ctx, item.GetName(), quota.InUse+quantity)
}

func (s *QuotaService[I]) checkLimit(q *quota.Quota, quantity int) error {
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
func (s *QuotaService[I]) SetLimit(ctx context.Context, item I, limit int) error {
	q, err := s.Storage.Get(ctx, item.GetName())
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < q.InUse {
		return quota.ErrLimitLowerThanAllocated
	}
	return s.Storage.SetLimit(ctx, item.GetName(), limit)
}

// Set redefines the inuse value for the named resource. This new value must be
// smaller than or equal to the current limit. It also must be a non negative
// number.
func (s *QuotaService[I]) Set(ctx context.Context, item I, inUse int) error {
	q, err := s.Storage.Get(ctx, item.GetName())
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
	return s.Storage.Set(ctx, item.GetName(), inUse)
}

func (s *QuotaService[I]) Get(ctx context.Context, item I) (*quota.Quota, error) {
	return s.Storage.Get(ctx, item.GetName())
}
