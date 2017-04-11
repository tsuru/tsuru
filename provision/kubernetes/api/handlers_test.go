// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ajg/form"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision/kubernetes/cluster"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

type S struct {
	user  *auth.User
	token auth.Token
	conn  *db.Storage
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "provision_kubernetes_api_tests_s")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	s.user, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "usr", permission.Permission{
		Scheme:  permission.PermKubernetesCluster,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) TestUpdateCluster(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Default:   true,
	}
	encoded, err := form.EncodeToString(kubeCluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest("POST", "/1.3/kubernetes/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := cluster.AllClusters()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 1)
	c.Assert(clusters[0].Name, check.Equals, "c1")
	c.Assert(clusters[0].Addresses, check.DeepEquals, []string{"addr1"})
	c.Assert(clusters[0].Default, check.Equals, true)
}

func (s *S) TestListClusters(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:       "c1",
		Addresses:  []string{"addr1"},
		CaCert:     []byte("testCA"),
		ClientCert: []byte("testCert"),
		ClientKey:  []byte("testKey"),
		Default:    true,
	}
	err := kubeCluster.Save()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.3/kubernetes/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	var retClusters []cluster.Cluster
	err = json.Unmarshal(recorder.Body.Bytes(), &retClusters)
	c.Assert(err, check.IsNil)
	c.Assert(retClusters, check.DeepEquals, []cluster.Cluster{{
		Name:       "c1",
		Addresses:  []string{"addr1"},
		CaCert:     []byte("testCA"),
		ClientCert: []byte("testCert"),
		Default:    true,
	}})
}

func (s *S) TestListClustersNoContent(c *check.C) {
	request, err := http.NewRequest("GET", "/1.3/kubernetes/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteClusterNotFound(c *check.C) {
	request, err := http.NewRequest("DELETE", "/1.3/kubernetes/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteCluster(c *check.C) {
	kubeCluster := cluster.Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Default:   true,
	}
	err := kubeCluster.Save()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.3/kubernetes/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := cluster.AllClusters()
	c.Assert(err, check.Equals, cluster.ErrNoCluster)
	c.Assert(clusters, check.HasLen, 0)
}
