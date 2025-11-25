// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multicluster

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/servicemanager"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

func Header(ctx context.Context, poolName string, existingHeader http.Header) (http.Header, error) {
	header := existingHeader
	if existingHeader == nil {
		header = http.Header{}
	}

	p, err := servicemanager.Pool.FindByName(ctx, poolName)
	if err != nil {
		return header, err
	}
	header.Set("X-Tsuru-Pool-Name", p.Name)
	header.Set("X-Tsuru-Pool-Provisioner", p.Provisioner)
	c, err := servicemanager.Cluster.FindByPool(ctx, p.Provisioner, p.Name)
	if err != nil {
		if err == provTypes.ErrNoCluster {
			return header, nil
		}
		return header, err
	}
	header.Set("X-Tsuru-Cluster-Name", c.Name)
	header.Set("X-Tsuru-Cluster-Provisioner", c.Provisioner)
	for _, addr := range c.Addresses {
		header.Add("X-Tsuru-Cluster-Addresses", addr)
	}

	if value, ok := c.CustomData["propagate-kubeconfig"]; ok && value == "true" {
		jsonData, err := json.Marshal(c.KubeConfig)
		if err != nil {
			return header, err
		}
		header.Set("X-Tsuru-Cluster-KubeConfig", base64.StdEncoding.EncodeToString(jsonData))
	}
	return header, nil
}
