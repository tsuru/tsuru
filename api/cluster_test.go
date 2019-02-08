// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
)

func (s *S) TestCreateCluster(c *check.C) {
	kubeCluster := provision.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
		ClientKey:   []byte("xyz"),
	}
	s.mockService.Cluster.OnFindByName = func(name string) (*provision.Cluster, error) {
		c.Assert(name, check.Equals, kubeCluster.Name)
		return nil, provision.ErrNoCluster
	}
	s.mockService.Cluster.OnCreate = func(cluster provision.Cluster) error {
		c.Assert(cluster.Writer, check.NotNil)
		cluster.Writer = nil
		c.Assert(cluster, check.DeepEquals, kubeCluster)
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
	kubeCluster := provision.Cluster{
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
	kubeCluster := provision.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Pools:       []string{"fakePool"},
	}
	s.mockService.Cluster.OnFindByName = func(name string) (*provision.Cluster, error) {
		c.Assert(name, check.Equals, kubeCluster.Name)
		return nil, provision.ErrNoCluster
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

func (s *S) TestCreateClusterJSON(c *check.C) {
	kubeCluster := provision.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
		ClientKey:   []byte("xyz"),
	}
	s.mockService.Cluster.OnFindByName = func(name string) (*provision.Cluster, error) {
		c.Assert(name, check.Equals, kubeCluster.Name)
		return nil, provision.ErrNoCluster
	}
	s.mockService.Cluster.OnCreate = func(cluster provision.Cluster) error {
		c.Assert(cluster.Writer, check.NotNil)
		cluster.Writer = nil
		c.Assert(cluster, check.DeepEquals, kubeCluster)
		return nil
	}
	encoded, err := json.Marshal(kubeCluster)
	c.Assert(err, check.IsNil)
	body := bytes.NewReader(encoded)
	request, err := http.NewRequest(http.MethodPost, "/1.3/provisioner/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestUpdateCluster(c *check.C) {
	kubeCluster := provision.Cluster{
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
	kubeCluster := provision.Cluster{
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
	kubeCluster := provision.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		CaCert:      []byte("testCA"),
		ClientCert:  []byte("testCert"),
		ClientKey:   []byte("testKey"),
		Provisioner: "fake",
		Default:     true,
	}
	s.mockService.Cluster.OnList = func() ([]provision.Cluster, error) {
		return []provision.Cluster{kubeCluster}, nil
	}
	request, err := http.NewRequest(http.MethodGet, "/1.3/provisioner/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	var retClusters []provision.Cluster
	err = json.Unmarshal(recorder.Body.Bytes(), &retClusters)
	c.Assert(err, check.IsNil)
	c.Assert(retClusters, check.DeepEquals, []provision.Cluster{{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		CaCert:      []byte("testCA"),
		ClientCert:  []byte("testCert"),
		Provisioner: "fake",
		Default:     true,
	}})
}

func (s *S) TestListClustersNoContent(c *check.C) {
	s.mockService.Cluster.OnList = func() ([]provision.Cluster, error) {
		return nil, provision.ErrNoCluster
	}
	request, err := http.NewRequest(http.MethodGet, "/1.3/provisioner/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteClusterNotFound(c *check.C) {
	s.mockService.Cluster.OnDelete = func(_ provision.Cluster) error {
		return provision.ErrClusterNotFound
	}
	request, err := http.NewRequest(http.MethodDelete, "/1.3/provisioner/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteCluster(c *check.C) {
	kubeCluster := provision.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Default:     true,
		Provisioner: "fake",
	}
	s.mockService.Cluster.OnDelete = func(clust provision.Cluster) error {
		c.Assert(clust.Name, check.Equals, kubeCluster.Name)
		return nil
	}
	request, err := http.NewRequest(http.MethodDelete, "/1.3/provisioner/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestListProvisioners(c *check.C) {
	request, err := http.NewRequest(http.MethodGet, "/1.7/provisioner", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	var retProvs []provisionerInfo
	err = json.Unmarshal(recorder.Body.Bytes(), &retProvs)
	c.Assert(err, check.IsNil)
	c.Assert(retProvs, check.DeepEquals, []provisionerInfo{
		{Name: "fake"},
	})
}
