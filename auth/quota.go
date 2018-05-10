// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

type authQuotaService struct {
	storage authTypes.QuotaStorage
}

func (s *authQuotaService) checkUserExists(email string) error {
	_, err := s.storage.FindByUserEmail(email)
	return err
}

func QuotaService() (authTypes.QuotaService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &authQuotaService{dbDriver.AuthQuotaStorage}, nil
}

// ReserveApp reserves an app for the user, reserving it in the database. It's
// used to reserve the app in the user quota, returning an error when there
// isn't any space available.
func (s *authQuotaService) ReserveApp(email string) error {
	quota, err := s.storage.FindByUserEmail(email)
	if err != nil {
		return err
	}
	if quota.Limit == quota.InUse {
		return &authTypes.QuotaExceededError{
			Available: 0, Requested: 1,
		}
	}
	err = s.storage.IncInUse(email, 1)
	if err != nil {
		return err
	}
	quota.InUse += 1
	return nil
}

// ReleaseApp releases an app from the user list, releasing the quota spot for
// another app.
func (s *authQuotaService) ReleaseApp(email string) error {
	quota, err := s.storage.FindByUserEmail(email)
	if err != nil {
		return err
	}
	if quota.InUse == 0 {
		return authTypes.ErrCantRelease
	}
	err = s.storage.IncInUse(email, -1)
	if err != nil {
		return err
	}
	quota.InUse -= 1
	return err
}

// ChangeQuota redefines the limit of the user. The new limit must be bigger
// than or equal to the current number of apps of the user. The new limit maybe
// smaller than 0, which mean that the user should have an unlimited number of
// apps.
func (s *authQuotaService) ChangeLimit(email string, limit int) error {
	quota, err := s.storage.FindByUserEmail(email)
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < quota.InUse {
		return authTypes.ErrLimitLowerThanAllocated
	}
	err = s.storage.SetLimit(email, limit)
	if err != nil {
		return err
	}
	quota.Limit = limit
	return err
}

func (s *authQuotaService) FindByUserEmail(email string) (*authTypes.Quota, error) {
	return s.storage.FindByUserEmail(email)
}
