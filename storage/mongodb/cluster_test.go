// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/storage/storagetest"
	"github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	authexec "k8s.io/client-go/plugin/pkg/client/auth/exec"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var _ = check.Suite(&storagetest.ClusterSuite{
	ClusterStorage: &clusterStorage{},
	SuiteHooks:     &mongodbBaseTest{},
})

type clusterSuite struct {
	storagetest.SuiteHooks
}

var _ = check.Suite(&clusterSuite{
	SuiteHooks: &mongodbBaseTest{name: "cluster-internal"},
})

func (s *clusterSuite) TestUpsertClusterWithExecConfigPreservesValidPluginPolicy(c *check.C) {
	storage := &clusterStorage{}
	cluster := provision.Cluster{
		Name: "clustername",
		KubeConfig: &provision.KubeConfig{
			AuthInfo: clientcmdapi.AuthInfo{
				Exec: &clientcmdapi.ExecConfig{
					Command:         "gke-gcloud-auth-plugin",
					APIVersion:      "client.authentication.k8s.io/v1beta1",
					InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode,
				},
			},
		},
	}

	err := storage.Upsert(context.Background(), cluster)
	c.Assert(err, check.IsNil)

	storedCluster, err := storage.FindByName(context.Background(), cluster.Name)
	c.Assert(err, check.IsNil)

	err = authexec.ValidatePluginPolicy(storedCluster.KubeConfig.AuthInfo.Exec.PluginPolicy)
	c.Assert(err, check.IsNil)
}
