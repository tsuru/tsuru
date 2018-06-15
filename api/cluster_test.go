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
	"github.com/tsuru/tsuru/servicemanager"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestCreateCluster(c *check.C) {
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
	}
	s.mockService.Cluster.OnFindByName = func(name string) (*provTypes.Cluster, error) {
		c.Assert(name, check.Equals, kubeCluster.Name)
		return nil, provTypes.ErrNoCluster
	}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest(http.MethodPost, "/1.3/provisioner/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestCreateClusterWithCreateData(c *check.C) {
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{},
		Provisioner: "fake",
		Default:     true,
		CreateData: map[string]string{
			"id":   "test1",
			"iaas": "test-iaas",
		},
	}
	s.mockService.Cluster.OnFindByName = func(name string) (*provTypes.Cluster, error) {
		c.Assert(name, check.Equals, kubeCluster.Name)
		return nil, provTypes.ErrNoCluster
	}
	s.mockService.Cluster.OnSave = func(clust provTypes.Cluster) error {
		c.Assert(clust.Addresses, check.DeepEquals, []string{"http://test1.somewhere.com:2375"})
		return nil
	}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest(http.MethodPost, "/1.3/provisioner/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestCreateClusterAlreadyExists(c *check.C) {
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Pools:       []string{"fakePool"},
	}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest(http.MethodPost, "/1.3/provisioner/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestCreateClusterWithNonExistentPool(c *check.C) {
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Pools:       []string{"fakePool"},
	}
	s.mockService.Cluster.OnFindByName = func(name string) (*provTypes.Cluster, error) {
		c.Assert(name, check.Equals, kubeCluster.Name)
		return nil, provTypes.ErrNoCluster
	}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest(http.MethodPost, "/1.3/provisioner/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestUpdateCluster(c *check.C) {
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
	}
	kubeCluster.CustomData = map[string]string{"c1": "v1"}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest(http.MethodPost, "/1.4/provisioner/clusters/c1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestUpdateClusterNonExistentPool(c *check.C) {
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
	}
	kubeCluster.Pools = []string{"fakePool"}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest(http.MethodPost, "/1.4/provisioner/clusters/c1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestListClusters(c *check.C) {
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		CaCert:      []byte("testCA"),
		ClientCert:  []byte("testCert"),
		ClientKey:   []byte("testKey"),
		Provisioner: "fake",
		Default:     true,
	}
	s.mockService.Cluster.OnList = func() ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{kubeCluster}, nil
	}
	request, err := http.NewRequest(http.MethodGet, "/1.3/provisioner/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	var retClusters []provTypes.Cluster
	err = json.Unmarshal(recorder.Body.Bytes(), &retClusters)
	c.Assert(err, check.IsNil)
	c.Assert(retClusters, check.DeepEquals, []provTypes.Cluster{{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		CaCert:      []byte("testCA"),
		ClientCert:  []byte("testCert"),
		Provisioner: "fake",
		Default:     true,
	}})
}

func (s *S) TestListClustersNoContent(c *check.C) {
	s.mockService.Cluster.OnList = func() ([]provTypes.Cluster, error) {
		return nil, provTypes.ErrNoCluster
	}
	request, err := http.NewRequest(http.MethodGet, "/1.3/provisioner/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteClusterNotFound(c *check.C) {
	s.mockService.Cluster.OnDelete = func(_ provTypes.Cluster) error {
		return provTypes.ErrClusterNotFound
	}
	request, err := http.NewRequest(http.MethodDelete, "/1.3/provisioner/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteCluster(c *check.C) {
	kubeCluster := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Default:     true,
		Provisioner: "fake",
	}
	err := servicemanager.Cluster.Save(kubeCluster)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodDelete, "/1.3/provisioner/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
}
