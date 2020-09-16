// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/storage"
	quotaTypes "github.com/tsuru/tsuru/types/quota"
)

func QuotaService() (quotaTypes.QuotaService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &quota.QuotaService{
		Storage: dbDriver.AppQuotaStorage,
	}, nil
}
