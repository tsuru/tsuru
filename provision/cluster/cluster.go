// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/storage"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/validation"
)

type ClusteredProvisioner interface {
	InitializeCluster(c *provTypes.Cluster) error
	ValidateCluster(c *provTypes.Cluster) error
	ClusterHelp() provTypes.ClusterHelpInfo
}

type clusterService struct {
	storage provTypes.ClusterStorage
}

var _ provTypes.ClusterService = &clusterService{}

func ClusterStorage() (provTypes.ClusterStorage, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return dbDriver.ClusterStorage, nil
}

func ClusterService() (provTypes.ClusterService, error) {
	storage, err := ClusterStorage()
	if err != nil {
		return nil, err
	}
	return &clusterService{
		storage: storage,
	}, nil
}

func (s *clusterService) Create(ctx context.Context, c provTypes.Cluster) error {
	err := s.validate(c, true)
	if err != nil {
		return err
	}

	return s.save(ctx, c, true)
}

func (s *clusterService) Update(ctx context.Context, c provTypes.Cluster) error {
	err := s.validate(c, false)
	if err != nil {
		return err
	}
	return s.save(ctx, c, false)
}

func (s *clusterService) save(ctx context.Context, c provTypes.Cluster, isNewCluster bool) error {
	err := s.storage.Upsert(ctx, c)
	if err != nil {
		return err
	}
	return s.initCluster(ctx, c, isNewCluster)
}

func (s *clusterService) List(ctx context.Context) ([]provTypes.Cluster, error) {
	return s.storage.FindAll(ctx)
}

func (s *clusterService) FindByName(ctx context.Context, name string) (*provTypes.Cluster, error) {
	return s.storage.FindByName(ctx, name)
}

func (s *clusterService) FindByProvisioner(ctx context.Context, prov string) ([]provTypes.Cluster, error) {
	return s.storage.FindByProvisioner(ctx, prov)
}

func (s *clusterService) FindByPools(ctx context.Context, prov string, pools []string) (map[string]provTypes.Cluster, error) {
	provClusters, err := s.FindByProvisioner(ctx, prov)
	if err != nil {
		return nil, err
	}
	result := make(map[string]provTypes.Cluster)
poolLoop:
	for _, pool := range pools {
		for _, cluster := range provClusters {
			if cluster.Default {
				result[pool] = cluster
			}
			for _, clusterPool := range cluster.Pools {
				if clusterPool == pool {
					result[pool] = cluster
					continue poolLoop
				}
			}
		}
		if _, ok := result[pool]; !ok {
			return nil, errors.Errorf("unable to find cluster for pool %q", pool)
		}
	}
	return result, nil
}

func (s *clusterService) FindByPool(ctx context.Context, prov, pool string) (*provTypes.Cluster, error) {
	return s.storage.FindByPool(ctx, prov, pool)
}

func (s *clusterService) Delete(ctx context.Context, c provTypes.Cluster) error {
	var err error
	c, err = s.updateClusterFromStorage(ctx, c)
	if err != nil {
		return err
	}

	return s.storage.Delete(ctx, c)
}

func (s *clusterService) validate(c provTypes.Cluster, isNewCluster bool) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "cluster name is mandatory"})
	}
	if isNewCluster && !validation.ValidateName(c.Name) {
		msg := "Invalid cluster name, cluster name should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return errors.WithStack(&tsuruErrors.ValidationError{Message: msg})
	}
	if c.Provisioner == "" {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: "provisioner name is mandatory"})
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
	prov, err := provision.Get(c.Provisioner)
	if err != nil {
		return errors.WithStack(&tsuruErrors.ValidationError{Message: fmt.Sprintf("provisioner error: %v", err)})
	}
	if clusterProv, ok := prov.(ClusteredProvisioner); ok {
		return clusterProv.ValidateCluster(&c)
	}
	return nil
}

func (s *clusterService) initCluster(ctx context.Context, c provTypes.Cluster, isNewCluster bool) error {
	prov, err := provision.Get(c.Provisioner)
	if err != nil {
		return err
	}

	if clusterProv, ok := prov.(ClusteredProvisioner); ok {
		err = clusterProv.InitializeCluster(&c)
	}
	return err
}

func (s *clusterService) updateClusterFromStorage(ctx context.Context, c provTypes.Cluster) (provTypes.Cluster, error) {
	updatedCluster, err := s.storage.FindByName(ctx, c.Name)
	if err != nil {
		return c, err
	}
	updatedCluster.Writer = c.Writer
	return *updatedCluster, nil
}
