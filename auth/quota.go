// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/quota"
)

type userQuotaService struct {
	storage quota.UserQuotaStorage
}

func (s *userQuotaService) checkUserExists(email string) error {
	_, err := s.storage.FindByUserEmail(email)
	return err
}

func QuotaService() (quota.UserQuotaService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &userQuotaService{dbDriver.UserQuotaStorage}, nil
}

// ReserveApp reserves an app for the user, reserving it in the database. It's
// used to reserve the app in the user quota, returning an error when there
// isn't any space available.
func (s *userQuotaService) ReserveApp(email string) error {
	q, err := s.storage.FindByUserEmail(email)
	if err != nil {
		return err
	}
	if q.Limit == q.InUse {
		return &quota.QuotaExceededError{
			Available: 0, Requested: 1,
		}
	}
	err = s.storage.IncInUse(email, 1)
	if err != nil {
		return err
	}
	q.InUse++
	return nil
}

// ReleaseApp releases an app from the user list, releasing the quota spot for
// another app.
func (s *userQuotaService) ReleaseApp(email string) error {
	q, err := s.storage.FindByUserEmail(email)
	if err != nil {
		return err
	}
	if q.InUse == 0 {
		return quota.ErrCantRelease
	}
	err = s.storage.IncInUse(email, -1)
	if err != nil {
		return err
	}
	q.InUse--
	return err
}

// ChangeQuota redefines the limit of the user. The new limit must be bigger
// than or equal to the current number of apps of the user. The new limit maybe
// smaller than 0, which mean that the user should have an unlimited number of
// apps.
func (s *userQuotaService) ChangeLimit(email string, limit int) error {
	q, err := s.storage.FindByUserEmail(email)
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < q.InUse {
		return quota.ErrLimitLowerThanAllocated
	}
	err = s.storage.SetLimit(email, limit)
	if err != nil {
		return err
	}
	q.Limit = limit
	return err
}

func (s *userQuotaService) FindByUserEmail(email string) (*quota.Quota, error) {
	return s.storage.FindByUserEmail(email)
}
