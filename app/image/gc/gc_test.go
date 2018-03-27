// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/globalsign/mgo"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	storage     *db.Storage
	user        *auth.User
	team        string
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "app_image_gc_tests")
	config.Set("docker:collection", "docker")
	config.Set("docker:repository-namespace", "tsuru")
	config.Set("routers:fake:type", "fake")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	var err error
	s.storage, err = db.Conn()
	c.Assert(err, check.IsNil)
	provision.DefaultProvisioner = "fake"
	app.AuthScheme = auth.ManagedScheme(native.NativeScheme{})
}

func (s *S) SetUpTest(c *check.C) {
	provisiontest.ProvisionerInstance.Reset()
	routertest.FakeRouter.Reset()
	s.user, _ = permissiontest.CustomUserWithPermission(c, app.AuthScheme, "majortom", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	s.team = "myteam"
	err := pool.AddPool(pool.AddPoolOptions{
		Name: "p1",
	})
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&s.mockService)
	plan := appTypes.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &plan, nil
	}
}

func (s *S) TearDownTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.storage.Apps().Database)
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
}

func (s *S) TestGCStartNothingToDo(c *check.C) {
	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err := gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
}

func (s *S) TestGCStartAppNotFound(c *check.C) {
	var regDeleteCalls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tags/list") {
			w.Write([]byte(`{"name":"tsuru/app-myapp","tags":[
				"v0","v1","v2","v3","v4","v5","v6","v7","v8","v9","v10","v11","my-custom-tag","v0-builder","v1-builder","v2-builder","v3-builder","v4-builder","v5-builder","v6-builder","v7-builder","v8-builder","v9-builder","v10-builder","v11-builder"
			]}`))
			return
		}
		if r.Method == "HEAD" {
			w.Header().Set("Docker-Content-Digest", r.URL.Path)
			return
		}
		if r.Method == "DELETE" {
			regDeleteCalls = append(regDeleteCalls, r.URL.Path)
		}
	}))
	u, _ := url.Parse(srv.URL)
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	defer srv.Close()
	err := image.AppendAppBuilderImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:my-custom-tag", u.Host))
	c.Assert(err, check.IsNil)
	for i := 0; i < 12; i++ {
		err = image.AppendAppImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:v%d", u.Host, i))
		c.Assert(err, check.IsNil)
		err = image.AppendAppBuilderImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:v%d-builder", u.Host, i))
		c.Assert(err, check.IsNil)
	}
	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	c.Assert(regDeleteCalls, check.DeepEquals, []string{
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v0",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v1",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v2",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v3",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v4",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v5",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v6",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v7",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v8",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v9",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v10",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v11",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/my-custom-tag",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v0-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v1-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v2-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v3-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v4-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v5-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v6-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v7-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v8-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v9-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v10-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v11-builder",
	})
	_, err = image.ListAppImages("myapp")
	c.Assert(err, check.Equals, mgo.ErrNotFound)
	_, err = image.ListAppBuilderImages("myapp")
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestGCStartWithApp(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	err := app.CreateApp(&app.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}, s.user)
	c.Assert(err, check.IsNil)
	var nodeDeleteCalls []string
	nodeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			nodeDeleteCalls = append(nodeDeleteCalls, r.URL.Path)
		}
	}))
	defer nodeSrv.Close()
	err = provisiontest.ProvisionerInstance.AddNode(provision.AddNodeOptions{
		Address: nodeSrv.URL,
		Pool:    "p1",
	})
	c.Assert(err, check.IsNil)
	var regDeleteCalls []string
	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Docker-Content-Digest", r.URL.Path)
			return
		}
		if r.Method == "DELETE" {
			regDeleteCalls = append(regDeleteCalls, r.URL.Path)
		}
	}))
	u, _ := url.Parse(registrySrv.URL)
	defer registrySrv.Close()
	err = image.AppendAppBuilderImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:my-custom-tag", u.Host))
	c.Assert(err, check.IsNil)
	for i := 0; i < 12; i++ {
		err = image.AppendAppImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:v%d", u.Host, i))
		c.Assert(err, check.IsNil)
		err = image.AppendAppBuilderImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:v%d-builder", u.Host, i))
		c.Assert(err, check.IsNil)
	}
	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	c.Assert(regDeleteCalls, check.DeepEquals, []string{
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v0",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v1",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v0-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v1-builder",
	})
	appImgs, err := image.ListAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(appImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:v2",
		u.Host + "/tsuru/app-myapp:v3",
		u.Host + "/tsuru/app-myapp:v4",
		u.Host + "/tsuru/app-myapp:v5",
		u.Host + "/tsuru/app-myapp:v6",
		u.Host + "/tsuru/app-myapp:v7",
		u.Host + "/tsuru/app-myapp:v8",
		u.Host + "/tsuru/app-myapp:v9",
		u.Host + "/tsuru/app-myapp:v10",
		u.Host + "/tsuru/app-myapp:v11",
	})
	builderImgs, err := image.ListAppBuilderImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(builderImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:my-custom-tag",
		u.Host + "/tsuru/app-myapp:v2-builder",
		u.Host + "/tsuru/app-myapp:v3-builder",
		u.Host + "/tsuru/app-myapp:v4-builder",
		u.Host + "/tsuru/app-myapp:v5-builder",
		u.Host + "/tsuru/app-myapp:v6-builder",
		u.Host + "/tsuru/app-myapp:v7-builder",
		u.Host + "/tsuru/app-myapp:v8-builder",
		u.Host + "/tsuru/app-myapp:v9-builder",
		u.Host + "/tsuru/app-myapp:v10-builder",
		u.Host + "/tsuru/app-myapp:v11-builder",
	})
	c.Assert(nodeDeleteCalls, check.DeepEquals, []string{
		"/images/" + u.Host + "/tsuru/app-myapp:v0",
		"/images/" + u.Host + "/tsuru/app-myapp:v1",
		"/images/" + u.Host + "/tsuru/app-myapp:v2",
		"/images/" + u.Host + "/tsuru/app-myapp:v3",
		"/images/" + u.Host + "/tsuru/app-myapp:v4",
		"/images/" + u.Host + "/tsuru/app-myapp:v5",
		"/images/" + u.Host + "/tsuru/app-myapp:v6",
		"/images/" + u.Host + "/tsuru/app-myapp:v7",
		"/images/" + u.Host + "/tsuru/app-myapp:v8",
		"/images/" + u.Host + "/tsuru/app-myapp:v9",
		"/images/" + u.Host + "/tsuru/app-myapp:v10",
		"/images/" + u.Host + "/tsuru/app-myapp:my-custom-tag",
		"/images/" + u.Host + "/tsuru/app-myapp:v0-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v1-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v2-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v3-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v4-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v5-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v6-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v7-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v8-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v9-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v10-builder",
	})
}

func (s *S) TestGCStartWithAppStressNotFound(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	err := app.CreateApp(&app.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}, s.user)
	c.Assert(err, check.IsNil)
	nodeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer nodeSrv.Close()
	err = provisiontest.ProvisionerInstance.AddNode(provision.AddNodeOptions{
		Address: nodeSrv.URL,
		Pool:    "p1",
	})
	c.Assert(err, check.IsNil)
	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	u, _ := url.Parse(registrySrv.URL)
	defer registrySrv.Close()
	err = image.AppendAppBuilderImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:my-custom-tag", u.Host))
	c.Assert(err, check.IsNil)
	for i := 0; i < 12; i++ {
		err = image.AppendAppImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:v%d", u.Host, i))
		c.Assert(err, check.IsNil)
		err = image.AppendAppBuilderImageName("myapp", fmt.Sprintf("%s/tsuru/app-myapp:v%d-builder", u.Host, i))
		c.Assert(err, check.IsNil)
	}
	nGoroutines := 10
	wg := sync.WaitGroup{}
	for i := 0; i < nGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gc := &imgGC{once: &sync.Once{}}
			gc.start()
			shutDownErr := gc.Shutdown(context.Background())
			c.Assert(shutDownErr, check.IsNil)
		}()
	}
	wg.Wait()
	appImgs, err := image.ListAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(appImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:v2",
		u.Host + "/tsuru/app-myapp:v3",
		u.Host + "/tsuru/app-myapp:v4",
		u.Host + "/tsuru/app-myapp:v5",
		u.Host + "/tsuru/app-myapp:v6",
		u.Host + "/tsuru/app-myapp:v7",
		u.Host + "/tsuru/app-myapp:v8",
		u.Host + "/tsuru/app-myapp:v9",
		u.Host + "/tsuru/app-myapp:v10",
		u.Host + "/tsuru/app-myapp:v11",
	})
	builderImgs, err := image.ListAppBuilderImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(builderImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:my-custom-tag",
		u.Host + "/tsuru/app-myapp:v2-builder",
		u.Host + "/tsuru/app-myapp:v3-builder",
		u.Host + "/tsuru/app-myapp:v4-builder",
		u.Host + "/tsuru/app-myapp:v5-builder",
		u.Host + "/tsuru/app-myapp:v6-builder",
		u.Host + "/tsuru/app-myapp:v7-builder",
		u.Host + "/tsuru/app-myapp:v8-builder",
		u.Host + "/tsuru/app-myapp:v9-builder",
		u.Host + "/tsuru/app-myapp:v10-builder",
		u.Host + "/tsuru/app-myapp:v11-builder",
	})
}
