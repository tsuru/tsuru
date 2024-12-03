// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"

	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
	quotaTypes "github.com/tsuru/tsuru/types/quota"
)

func QuotaService() (quotaTypes.QuotaService[*appTypes.App], error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &appQuotaService{
		Storage: dbDriver.AppQuotaStorage,
	}, nil
}

type appQuotaService struct {
	Storage quotaTypes.QuotaStorage
}

// Inc implements Inc method from QuotaService interface
func (s *appQuotaService) Inc(ctx context.Context, app *appTypes.App, quantity int) error {
	quota, err := s.Get(ctx, app)
	if err != nil {
		return err
	}
	err = s.checkLimit(quota, quantity)
	if err != nil {
		return err
	}
	return s.Storage.Set(ctx, app.Name, quota.InUse+quantity)
}

func (s *appQuotaService) checkLimit(q *quotaTypes.Quota, quantity int) error {
	if !q.IsUnlimited() && q.InUse+quantity > q.Limit {
		return &quotaTypes.QuotaExceededError{
			Available: uint(q.Limit - q.InUse),
			Requested: uint(quantity),
		}
	}
	if q.InUse+quantity < 0 {
		return quotaTypes.ErrNotEnoughReserved
	}
	return nil
}

// SetLimit redefines the limit of the named resource. The new limit must be
// bigger than or equal to the current isUse. The new
// limit may be smaller than 0, which means that the app should have an
// unlimited number of units. SetLimit implements SetLimit method from
// QuotaService interface
func (s *appQuotaService) SetLimit(ctx context.Context, app *appTypes.App, limit int) error {
	q, err := s.Get(ctx, app)
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < q.InUse {
		return quotaTypes.ErrLimitLowerThanAllocated
	}
	return s.Storage.SetLimit(ctx, app.Name, limit)
}

// Set redefines the inuse value for the named resource. This new value must be
// smaller than or equal to the current limit. It also must be a non negative
// number.
func (s *appQuotaService) Set(ctx context.Context, app *appTypes.App, inUse int) error {
	q, err := s.Storage.Get(ctx, app.Name)
	if err != nil {
		return err
	}
	if inUse < 0 {
		return quotaTypes.ErrLessThanZero
	}
	if !q.IsUnlimited() && inUse > q.Limit {
		return &quotaTypes.QuotaExceededError{
			Requested: uint(inUse),
			Available: uint(q.Limit),
		}
	}
	return s.Storage.Set(ctx, app.Name, inUse)
}

func (s *appQuotaService) Get(ctx context.Context, app *appTypes.App) (*quotaTypes.Quota, error) {
	q, err := s.Storage.Get(ctx, app.Name)
	if err != nil {
		return nil, err
	}

	q.InUse, err = GetQuotaInUse(ctx, app)
	if err != nil {
		return nil, err
	}
	return q, nil
}
