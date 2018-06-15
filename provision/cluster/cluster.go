// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"strings"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/storage"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/validation"
)

type InitClusterProvisioner interface {
	InitializeCluster(c *provTypes.Cluster) error
}

type clusterService struct {
	storage provTypes.ClusterStorage
}

func ClusterService() (provTypes.ClusterService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &clusterService{
		storage: dbDriver.ClusterStorage,
	}, nil
}

func (s *clusterService) Save(c provTypes.Cluster) error {
	err := s.validate(c)
	if err != nil {
		return err
	}
	return s.storage.Upsert(c)
}

func (s *clusterService) List() ([]provTypes.Cluster, error) {
	return s.storage.FindAll()
}

func (s *clusterService) FindByName(name string) (*provTypes.Cluster, error) {
	return s.storage.FindByName(name)
}

func (s *clusterService) FindByProvisioner(prov string) ([]provTypes.Cluster, error) {
	return s.storage.FindByProvisioner(prov)
}

func (s *clusterService) FindByPool(prov, pool string) (*provTypes.Cluster, error) {
	return s.storage.FindByPool(prov, pool)
}

func (s *clusterService) Delete(c provTypes.Cluster) error {
	return s.storage.Delete(c)
}

func (s *clusterService) validate(c provTypes.Cluster) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "cluster name is mandatory"})
	}
	if !validation.ValidateName(c.Name) {
		msg := "Invalid cluster name, cluster name should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return errors.WithStack(&tsuruErrors.ValidationError{Message: msg})
	}
	if c.Provisioner == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "provisioner name is mandatory"})
	}
	prov, err := provision.Get(c.Provisioner)
	if err != nil {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: err.Error()})
	}
	if len(c.Pools) > 0 {
		if c.Default {
			return errors.WithStack(&tsuruErrors.ValidationError{Message: "cannot have both pools and default set"})
		}
	} else {
		if !c.Default {
			return errors.WithStack(&tsuruErrors.ValidationError{Message: "either default or a list of pools must be set"})
		}
	}
	if clusterProv, ok := prov.(InitClusterProvisioner); ok {
		err = clusterProv.InitializeCluster(&c)
		if err != nil {
			return err
		}
	}
	return nil
}
