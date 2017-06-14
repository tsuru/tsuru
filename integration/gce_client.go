// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"

	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"

	"golang.org/x/net/context"
)

const (
	prodAddr  = "https://container.googleapis.com/"
	userAgent = "gcloud-golang-container/20151008"
)

type gceClient struct {
	projectID string
	context   context.Context
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
		context:   ctx,
		svc:       svc,
	}
	return c, nil
}

func (c *gceClient) createCluster(name, zone string, nodeCount int64) error {
	config := &container.CreateClusterRequest{
		Cluster: &container.Cluster{Name: name, InitialNodeCount: nodeCount},
	}
	_, err := c.svc.Projects.Zones.Clusters.Create(c.projectID, zone, config).Context(c.context).Do()
	return err
}

func (c *gceClient) describeCluster(name, zone string) (*container.Cluster, error) {
	return c.svc.Projects.Zones.Clusters.Get(c.projectID, zone, name).Do()
}

func (c *gceClient) deleteCluster(name, zone string) error {
	_, err := c.svc.Projects.Zones.Clusters.Delete(c.projectID, zone, name).Do()
	return err
}
