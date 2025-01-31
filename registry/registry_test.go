// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	registrytest "github.com/tsuru/tsuru/registry/testing"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	"go.mongodb.org/mongo-driver/bson/primitive"
	check "gopkg.in/check.v1"
)

type S struct {
	server      *registrytest.RegistryServer
	cluster     *provisionTypes.Cluster
	mockService servicemock.MockService
}

var suiteInstance = &S{}
var _ = check.Suite(suiteInstance)

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "app_image_repository_tests")
	s.server, err = registrytest.NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	provision.DefaultProvisioner = "fake"
	storagev2.Reset()

}

func (s *S) SetUpTest(c *check.C) {
	var err error
	storagev2.ClearAllCollections(nil)
	s.cluster = &provisionTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
		CustomData:  map[string]string{"registry": s.server.Addr() + "/tsuru"},
	}
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-default",
		Default:     true,
		Provisioner: "fake",
	})
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&s.mockService)
	s.mockService.Cluster.OnFindByName = func(name string) (*provisionTypes.Cluster, error) {
		c.Assert(name, check.Equals, s.cluster.Name)
		return nil, provisionTypes.ErrNoCluster
	}
	s.mockService.Cluster.OnList = func() ([]provisionTypes.Cluster, error) {
		return []provisionTypes.Cluster{*s.cluster}, nil
	}
	s.mockService.Cluster.OnFindByProvisioner = func(provisioner string) ([]provisionTypes.Cluster, error) {
		c.Assert(provisioner, check.Equals, s.cluster.Provisioner)
		return []provisionTypes.Cluster{*s.cluster}, nil
	}
	s.mockService.App.OnRegistry = func(app *appTypes.App) (imgTypes.ImageRegistry, error) {
		registry := s.cluster.CustomData["registry"]
		return imgTypes.ImageRegistry(registry), nil
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.server.Stop()
	storagev2.ClearAllCollections(nil)
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Reset()
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRegistryRemoveAppImagesIgnoreImageNotFound(c *check.C) {
	err := RemoveAppImages(context.TODO(), "test")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRegistryRemoveAppImagesErrorErrDeleteDisabled(c *check.C) {
	fakeApp := provisiontest.NewFakeApp("test", "go", 0)
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App:     fakeApp,
		EventID: primitive.NewObjectID().Hex(),
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	s.server.SetStorageDelete(false)
	err = RemoveAppImages(context.TODO(), "test")
	c.Assert(errors.Cause(err), check.Equals, ErrDeleteDisabled)
}

func (s *S) TestRegistryRemoveAppImages(c *check.C) {
	for i := 1; i <= 2; i++ {
		fakeApp := provisiontest.NewFakeApp("test", "go", 0)
		version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
			App:     fakeApp,
			EventID: primitive.NewObjectID().Hex(),
		})
		c.Assert(err, check.IsNil)
		err = version.CommitBaseImage()
		c.Assert(err, check.IsNil)
		err = version.CommitSuccessful()
		c.Assert(err, check.IsNil)
	}
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveAppImages(context.TODO(), "test")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 0)
}

func (s *S) TestRegistryRemoveAppImagesDetermined(c *check.C) {
	for i := 1; i <= 2; i++ {
		fakeApp := provisiontest.NewFakeApp("test", "go", 0)
		version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
			App:     fakeApp,
			EventID: primitive.NewObjectID().Hex(),
		})
		c.Assert(err, check.IsNil)
		err = version.CommitBaseImage()
		c.Assert(err, check.IsNil)
		err = version.CommitSuccessful()
		c.Assert(err, check.IsNil)
	}
	for i := 1; i <= 2; i++ {
		fakeApp := provisiontest.NewFakeApp("not-removed", "go", 0)
		version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
			App:     fakeApp,
			EventID: primitive.NewObjectID().Hex(),
		})
		c.Assert(err, check.IsNil)
		err = version.CommitBaseImage()
		c.Assert(err, check.IsNil)
		err = version.CommitSuccessful()
		c.Assert(err, check.IsNil)
	}
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "test1", "v2": "test2"}})
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-not-removed", Tags: map[string]string{"v1": "no-remove1", "v2": "no-remove2"}})
	c.Assert(s.server.Repos, check.HasLen, 2)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	c.Assert(s.server.Repos[1].Tags, check.HasLen, 2)
	c.Assert(s.server.Repos[0].Tags["v1"], check.Equals, "test1")
	c.Assert(s.server.Repos[0].Tags["v2"], check.Equals, "test2")
	c.Assert(s.server.Repos[1].Tags["v1"], check.Equals, "no-remove1")
	c.Assert(s.server.Repos[1].Tags["v2"], check.Equals, "no-remove2")
	err := RemoveAppImages(context.TODO(), "test")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 2)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 0)
	c.Assert(s.server.Repos[1].Tags, check.HasLen, 2)
	c.Assert(s.server.Repos[1].Tags["v1"], check.Equals, "no-remove1")
	c.Assert(s.server.Repos[1].Tags["v2"], check.Equals, "no-remove2")
}

func (s *S) TestRegistryRemoveImage(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v1")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageWithAuth(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}, Username: "user", Password: "pwd"})
	encoded := base64.StdEncoding.EncodeToString([]byte("user:pwd"))
	s.cluster.CustomData["docker-config-json"] = `{"auths": {"` + s.server.Addr() + `": {"auth": ` + strconv.Quote(encoded) + `}}}`
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v1")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageWithAuthToken(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}, Username: "user", Password: "pwd", Token: "mytoken", Expire: 30})
	s.server.SetTokenAuth(true)
	encoded := base64.StdEncoding.EncodeToString([]byte("user:pwd"))
	s.cluster.CustomData["docker-config-json"] = `{"auths": {"` + s.server.Addr() + `": {"auth": ` + strconv.Quote(encoded) + `}}}`
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v1")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageWithAuthBadCredentials(c *check.C) {
	encoded := base64.StdEncoding.EncodeToString([]byte("user:badpwd"))
	s.cluster.CustomData["docker-config-json"] = `{"auths": {"` + s.server.Addr() + `": {"auth": ` + strconv.Quote(encoded) + `}}}`
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}, Username: "user", Password: "pwd"})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v1")
	c.Assert(err, check.NotNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
}

func (s *S) TestRegistryRemoveImageNoRegistry(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage(context.TODO(), "tsuru/app-test:v1")
	c.Assert(err, check.ErrorMatches, `.*invalid empty registry.*`)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageUnknownRegistry(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage(context.TODO(), "fake-registry:5000/tsuru/app-test:v1")
	c.Assert(err, check.ErrorMatches, `.*failed to get digest for image.*`)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageUnknownTag(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v0")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "failed to get digest for image "+s.server.Addr()+"/tsuru/app-test:v0 on registry: digest not found")
	c.Assert(errors.Cause(err), check.Equals, ErrDigestNotFound)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageEmpty(c *check.C) {
	err := RemoveImage(context.TODO(), "")
	c.Assert(err, check.ErrorMatches, `invalid empty image name`)
}

func (s *S) TestRegistryRemoveImageDigestNotFound(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Docker-Content-Digest", "xyz")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	u, _ := url.Parse(srv.URL)
	err := RemoveImage(context.TODO(), u.Host+"/tsuru/app-test:v1")
	c.Assert(err, check.ErrorMatches, `failed to remove image .* on registry: image not found`)
	c.Assert(errors.Cause(err), check.Equals, ErrImageNotFound)
}

func (s *S) TestRegistryRemoveImageEmptyDigest(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	u, _ := url.Parse(srv.URL)
	err := RemoveImage(context.TODO(), u.Host+"/tsuru/app-test:v1")
	c.Assert(err, check.ErrorMatches, `.*empty digest returned for image tsuru/app-test:v1.*`)
}

func (s *S) TestDockerRegistryDoRequest(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	r := dockerRegistry{
		registry: srv.URL,
		client:   srv.Client(),
	}
	rsp, err := r.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
}

func (s *S) TestDockerRegistryDoRequestTLS(c *check.C) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	r := dockerRegistry{
		registry: srv.URL,
		client:   srv.Client(),
	}
	rsp, err := r.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
}
