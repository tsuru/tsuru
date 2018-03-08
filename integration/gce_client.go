// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
)

const (
	prodAddr  = "https://container.googleapis.com/"
	userAgent = "gcloud-golang-container/20151008"
)

type gceClient struct {
	projectID string
	svc       *container.Service
}

func newClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*gceClient, error) {
	o := []option.ClientOption{
		option.WithEndpoint(prodAddr),
		option.WithScopes(container.CloudPlatformScope),
		option.WithUserAgent(userAgent),
	}
	o = append(o, opts...)
	httpClient, endpoint, err := transport.NewHTTPClient(ctx, o...)
	if err != nil {
		return nil, fmt.Errorf("dialing: %v", err)
	}
	svc, err := container.New(httpClient)
	if err != nil {
		return nil, fmt.Errorf("constructing container client: %v", err)
	}
	svc.BasePath = endpoint
	c := &gceClient{
		projectID: projectID,
		svc:       svc,
	}
	return c, nil
}

func (c *gceClient) createCluster(ctx context.Context, name, zone, machineType string, nodeCount int64) error {
	serverConfig, err := c.svc.Projects.Zones.GetServerconfig(c.projectID, zone).Context(ctx).Do()
	if err != nil {
		return err
	}
	var version string
	if len(serverConfig.ValidMasterVersions) > 0 {
		sort.Sort(sort.Reverse(sort.StringSlice(serverConfig.ValidMasterVersions)))
		version = serverConfig.ValidMasterVersions[0]
	}
	config := &container.CreateClusterRequest{
		Cluster: &container.Cluster{
			Name:                  name,
			InitialNodeCount:      nodeCount,
			InitialClusterVersion: version,
			NodeConfig:            &container.NodeConfig{MachineType: machineType},
		},
	}
	_, err = c.svc.Projects.Zones.Clusters.Create(c.projectID, zone, config).Context(ctx).Do()
	return err
}

func (c *gceClient) describeCluster(ctx context.Context, name, zone string) (*container.Cluster, error) {
	return c.svc.Projects.Zones.Clusters.Get(c.projectID, zone, name).Context(ctx).Do()
}

func (c *gceClient) deleteCluster(ctx context.Context, name, zone string) error {
	_, err := c.svc.Projects.Zones.Clusters.Delete(c.projectID, zone, name).Context(ctx).Do()
	return err
}
