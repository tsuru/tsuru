// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision/cluster"
	"gopkg.in/check.v1"
)

func (s *S) TestCreateCluster(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
	}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest("POST", "/1.3/provisioner/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := cluster.AllClusters()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 1)
	c.Assert(clusters[0].Name, check.Equals, "c1")
	c.Assert(clusters[0].Addresses, check.DeepEquals, []string{"addr1"})
	c.Assert(clusters[0].Default, check.Equals, true)
}

func (s *S) TestCreateClusterWithCreateData(c *check.C) {
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	kubeCluster := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{},
		Provisioner: "fake",
		Default:     true,
		CreateData: map[string]string{
			"id":   "test1",
			"iaas": "test-iaas",
		},
	}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest("POST", "/1.3/provisioner/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := cluster.AllClusters()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 1)
	c.Assert(clusters[0].Name, check.Equals, "c1")
	c.Assert(clusters[0].Addresses, check.DeepEquals, []string{"http://test1.somewhere.com:2375"})
	c.Assert(clusters[0].Default, check.Equals, true)
}

func (s *S) TestUpdateCluster(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
	}
	err := kubeCluster.Save()
	c.Assert(err, check.IsNil)
	kubeCluster.CustomData = map[string]string{"c1": "v1"}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest("POST", "/1.4/provisioner/clusters/c1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := cluster.AllClusters()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 1)
	c.Assert(clusters[0].Name, check.Equals, "c1")
	c.Assert(clusters[0].Addresses, check.DeepEquals, []string{"addr1"})
	c.Assert(clusters[0].Default, check.Equals, true)
	c.Assert(clusters[0].CustomData, check.DeepEquals, map[string]string{"c1": "v1"})
}

func (s *S) TestListClusters(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		CaCert:      []byte("testCA"),
		ClientCert:  []byte("testCert"),
		ClientKey:   []byte("testKey"),
		Provisioner: "fake",
		Default:     true,
	}
	err := kubeCluster.Save()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.3/provisioner/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	var retClusters []cluster.Cluster
	err = json.Unmarshal(recorder.Body.Bytes(), &retClusters)
	c.Assert(err, check.IsNil)
	c.Assert(retClusters, check.DeepEquals, []cluster.Cluster{{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		CaCert:      []byte("testCA"),
		ClientCert:  []byte("testCert"),
		Provisioner: "fake",
		Default:     true,
	}})
}

func (s *S) TestListClustersNoContent(c *check.C) {
	request, err := http.NewRequest("GET", "/1.3/provisioner/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteClusterNotFound(c *check.C) {
	request, err := http.NewRequest("DELETE", "/1.3/provisioner/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteCluster(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Default:     true,
		Provisioner: "fake",
	}
	err := kubeCluster.Save()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.3/provisioner/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := cluster.AllClusters()
	c.Assert(err, check.Equals, cluster.ErrNoCluster)
	c.Assert(clusters, check.HasLen, 0)
}

func (s *S) TestCreateClusterWithNonexistentPool(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Pools:       []string{"fakePool"},
	}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest("POST", "/1.3/provisioner/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := cluster.AllClusters()
	c.Assert(err, check.Equals, cluster.ErrNoCluster)
	c.Assert(clusters, check.HasLen, 0)
}

func (s *S) TestAddClusterToNonexistentPool(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
	}
	err := kubeCluster.Save()
	c.Assert(err, check.IsNil)
	kubeCluster.Pools = []string{"fakePool"}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest("POST", "/1.4/provisioner/clusters/c1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := cluster.AllClusters()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 1)
	c.Assert(clusters[0].Name, check.Equals, "c1")
	c.Assert(clusters[0].Addresses, check.DeepEquals, []string{"addr1"})
	c.Assert(clusters[0].Pools, check.IsNil)
	c.Assert(clusters[0].Default, check.Equals, true)
}
