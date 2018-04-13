// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	authTypes "github.com/tsuru/tsuru/types/auth"
)

type authQuotaService struct {
	storage authTypes.AuthQuotaStorage
}

// ReserveApp reserves an app for the user, reserving it in the database. It's
// used to reserve the app in the user quota, returning an error when there
// isn't any space available.
func (s *authQuotaService) ReserveApp(email string, quota *authTypes.AuthQuota) error {
	err := checkUserExists(email)
	if err != nil {
		return err
	}
	if quota.Limit == quota.InUse {
		return &authTypes.AuthQuotaExceededError{
			Available: 0, Requested: 1,
		}
	}
	err = s.storage.IncInUse(email, quota, 1)
	if err != nil {
		return err
	}
	quota.InUse += 1
	return nil
}

func checkUserExists(email string) error {
	_, err := GetUserByEmail(email)
	if err != nil {
		return err
	}
	return nil
}

// ReleaseApp releases an app from the user list, releasing the quota spot for
// another app.
func (s *authQuotaService) ReleaseApp(email string, quota *authTypes.AuthQuota) error {
	err := checkUserExists(email)
	if err != nil {
		return err
	}
	if quota.InUse == 0 {
		return authTypes.ErrCantRelease
	}
	err = s.storage.IncInUse(email, quota, -1)
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
func (s *authQuotaService) ChangeQuota(email string, quota *authTypes.AuthQuota, limit int) error {
	err := checkUserExists(email)
	if err != nil {
		return err
	}
	if limit < 0 {
		limit = -1
	} else if limit < quota.InUse {
		return authTypes.ErrLimitLowerThanAllocated
	}
	err = s.storage.SetLimit(email, quota, limit)
	if err != nil {
		return err
	}
	quota.Limit = limit
	return err
}
