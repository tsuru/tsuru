// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision/kubernetes"
	"gopkg.in/check.v1"
)

var _ = check.Suite(&S{})

type S struct {
	conn  *db.Storage
	user  *auth.User
	token auth.Token
}

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpTest(c *check.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	err = dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	s.user, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "usr", permission.Permission{
		Scheme:  permission.PermKubernetesCluster,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
}

func (s *S) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *S) TestUpdateCluster(c *check.C) {
	cluster := kubernetes.Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Default:   true,
	}
	encoded, err := form.EncodeToString(cluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest("POST", "/1.3/kubernetes/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := kubernetes.AllClusters()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.DeepEquals, []kubernetes.Cluster{cluster})
}

func (s *S) TestListClusters(c *check.C) {
	cluster := kubernetes.Cluster{
		Name:       "c1",
		Addresses:  []string{"addr1"},
		CaCert:     []byte("cacert"),
		ClientCert: []byte("clientcert"),
		ClientKey:  []byte("clientkey"),
		Default:    true,
	}
	err := cluster.Save()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.3/kubernetes/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	var retClusters []kubernetes.Cluster
	err = json.Unmarshal(recorder.Body.Bytes(), &retClusters)
	c.Assert(err, check.IsNil)
	c.Assert(retClusters, check.HasLen, 1)
	c.Assert(retClusters[0].ClientKey, check.HasLen, 0)
	cluster.ClientKey = nil
	c.Assert(retClusters[0], check.DeepEquals, cluster)
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
	cluster := kubernetes.Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Default:   true,
	}
	err := cluster.Save()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.3/kubernetes/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := kubernetes.AllClusters()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 0)
}
