// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"time"

	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/cache"
	"github.com/tsuru/tsuru/types/service"
)

const defaultExpiration = 15 * time.Minute

var _ service.ServiceBrokerCatalogCacheService = &serviceBrokerCatalogCacheService{}

type serviceBrokerCatalogCacheService struct {
	storage cache.CacheStorage
}

func CatalogCacheService() (service.ServiceBrokerCatalogCacheService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &serviceBrokerCatalogCacheService{dbDriver.ServiceBrokerCatalogCacheStorage}, nil
}

func (s *serviceBrokerCatalogCacheService) Save(brokerName string, catalog service.BrokerCatalog) error {
	b, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	entry := cache.CacheEntry{
		Key:      brokerName,
		Value:    string(b),
		ExpireAt: s.expirationTime(brokerName),
	}
	return s.storage.Put(entry)
}

func (s *serviceBrokerCatalogCacheService) Load(brokerName string) (*service.BrokerCatalog, error) {
	entry, err := s.storage.Get(brokerName)
	if err != nil {
		return nil, err
	}

	var catalog service.BrokerCatalog
	err = json.Unmarshal([]byte(entry.Value), &catalog)
	if err != nil {
		return nil, err
	}
	return &catalog, nil
}

func (s *serviceBrokerCatalogCacheService) expirationTime(brokerName string) time.Time {
	expiration := defaultExpiration
	sb, err := servicemanager.ServiceBroker.Find(brokerName)
	if err == nil && sb.Config.CacheExpiration != nil {
		expiration = *sb.Config.CacheExpiration
	}
	return time.Now().Add(expiration)
}
