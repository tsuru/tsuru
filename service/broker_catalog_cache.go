// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"time"

	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/cache"
	"github.com/tsuru/tsuru/types/service"
)

const defaultExpiration = 15 * time.Minute

var _ ServiceBrokerCatalogCacheService = &serviceBrokerCatalogCacheService{}

type ServiceBrokerCatalogCacheService interface {
	Save(string, service.BrokerCatalog) error
	Load(string) (*service.BrokerCatalog, error)
}

type serviceBrokerCatalogCacheService struct {
	storage cache.CacheStorage
}

func CatalogCacheService() (ServiceBrokerCatalogCacheService, error) {
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
		ExpireAt: time.Now().Add(defaultExpiration),
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
