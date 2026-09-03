// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
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

func TestUpsertClusterWithExecConfigPreservesValidPluginPolicy(t *testing.T) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=150")
	config.Set("database:name", "tsuru_storage_mongodb_test_cluster_internal")
	storagev2.Reset()
	require.NoError(t, storagev2.ClearAllCollections(nil))
	t.Cleanup(func() {
		err := storagev2.ClearAllCollections(nil)
		storagev2.Reset()
		require.NoError(t, err)
	})

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
	require.NoError(t, err)

	storedCluster, err := storage.FindByName(context.Background(), cluster.Name)
	require.NoError(t, err)
	require.NotNil(t, storedCluster.KubeConfig)
	require.NotNil(t, storedCluster.KubeConfig.AuthInfo.Exec)

	err = authexec.ValidatePluginPolicy(storedCluster.KubeConfig.AuthInfo.Exec.PluginPolicy)
	assert.NoError(t, err)
}
