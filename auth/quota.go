// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
	quotaTypes "github.com/tsuru/tsuru/types/quota"
)

func UserQuotaService() (quotaTypes.LegacyQuotaService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &quota.QuotaService[quotaTypes.QuotaItem]{Storage: dbDriver.UserQuotaStorage}, nil
}

func TeamQuotaService() (quotaTypes.QuotaService[*authTypes.Team], error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &quota.QuotaService[*authTypes.Team]{Storage: dbDriver.TeamQuotaStorage}, nil
}
