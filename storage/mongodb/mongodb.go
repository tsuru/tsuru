// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/storage"
)

func init() {
	mongodbDriver := storage.DbDriver{
		TeamStorage:                      &TeamStorage{},
		PlatformStorage:                  &PlatformStorage{},
		PlatformImageStorage:             &PlatformImageStorage{},
		PlanStorage:                      &PlanStorage{},
		AppCacheStorage:                  appCacheStorage(),
		TeamTokenStorage:                 &teamTokenStorage{},
		UserQuotaStorage:                 authQuotaStorage(),
		AppQuotaStorage:                  appQuotaStorage(),
		WebhookStorage:                   &webhookStorage{},
		ClusterStorage:                   &clusterStorage{},
		ServiceBrokerStorage:             &serviceBrokerStorage{},
		ServiceBrokerCatalogCacheStorage: serviceBrokerCatalogCacheStorage(),
		AppLogStorage:                    &applogStorage{},
		InstanceTrackerStorage:           &instanceTrackerStorage{},
		AppVersionStorage:                &appVersionStorage{},
		DynamicRouterStorage:             &dynamicRouterStorage{},
		AuthGroupStorage:                 &authGroupStorage{},
		PoolStorage:                      &PoolStorage{},
	}
	storage.RegisterDbDriver("mongodb", mongodbDriver)
}
