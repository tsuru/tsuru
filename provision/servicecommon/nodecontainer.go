// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"fmt"
	"io"
	"sort"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/nodecontainer"
)

type NodeContainerManager interface {
	DeployNodeContainer(conf *nodecontainer.NodeContainerConfig, pool string, filter PoolFilter, placementOnly bool) error
}

type PoolFilter struct {
	Include []string
	Exclude []string
}

func EnsureNodeContainersCreated(manager NodeContainerManager, w io.Writer) error {
	names, err := nodecontainer.AllNodeContainersNames()
	if err != nil {
		return errors.WithStack(err)
	}
	for _, n := range names {
		err = upgradeNodeContainer(manager, n, "", true, w)
		if err != nil {
			return err
		}
	}
	return nil
}

func UpgradeNodeContainer(manager NodeContainerManager, name, poolToUpgrade string, w io.Writer) error {
	return upgradeNodeContainer(manager, name, poolToUpgrade, false, w)
}

func upgradeNodeContainer(manager NodeContainerManager, name, poolToUpgrade string, ensureOnly bool, w io.Writer) error {
	var excludeAllPools []string
	poolsToRun := []string{"", poolToUpgrade}
	poolMap, err := nodecontainer.LoadNodeContainersForPools(name)
	if err != nil {
		return errors.WithStack(err)
	}
	if poolToUpgrade == "" {
		poolsToRun = make([]string, 0, len(poolMap))
	}
	for poolName := range poolMap {
		if poolToUpgrade == "" {
			poolsToRun = append(poolsToRun, poolName)
		}
		if poolName != "" {
			excludeAllPools = append(excludeAllPools, poolName)
		}
	}
	sort.Strings(excludeAllPools)
	sort.Strings(poolsToRun)
	multiErr := tsuruErrors.NewMultiError()
	for _, poolName := range poolsToRun {
		config, err := nodecontainer.LoadNodeContainer(poolName, name)
		if err != nil {
			multiErr.Add(err)
			continue
		}
		if !config.Valid() {
			fmt.Fprintf(w, "skipping node container %q [%q], invalid config\n", name, poolName)
			continue
		}
		fmt.Fprintf(w, "upserting node container %q [%q]\n", name, poolName)
		var filter PoolFilter
		if poolName == "" {
			filter.Exclude = excludeAllPools
		} else {
			filter.Include = []string{poolName}
		}
		err = manager.DeployNodeContainer(config, poolName, filter, ensureOnly || (poolName == "" && poolToUpgrade != ""))
		if err != nil {
			multiErr.Add(err)
		}
	}
	if multiErr.Len() > 0 {
		return multiErr
	}
	return nil
}
