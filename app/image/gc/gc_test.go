// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
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
		Context: permission.Context(permTypes.CtxGlobal, ""),
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
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.storage.Apps().Database)
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
}

func insertTestVersions(c *check.C, a provision.App) {
	version, err := servicemanager.AppVersion.NewAppVersion(appTypes.NewVersionArgs{
		App:            a,
		CustomBuildTag: "my-custom-tag",
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBuildImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	desiredNumberOfVersions := 12
	for i := 1; i <= desiredNumberOfVersions; i++ {
		version, err = servicemanager.AppVersion.NewAppVersion(appTypes.NewVersionArgs{
			App: a,
		})
		c.Assert(err, check.IsNil)
		err = version.CommitBuildImage()
		c.Assert(err, check.IsNil)
		err = version.CommitBaseImage()
		c.Assert(err, check.IsNil)
		err = version.CommitSuccessful()
		c.Assert(err, check.IsNil)
	}
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
	fakeApp := provisiontest.NewFakeApp("myapp", "go", 0)
	insertTestVersions(c, fakeApp)
	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err := gc.Shutdown(context.Background())
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
	versions, err := servicemanager.AppVersion.AppVersions(fakeApp)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)
	c.Assert(len(versions.Versions), check.Equals, 0)
}

func (s *S) TestGCStartWithApp(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &app.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(a, s.user)
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

	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	insertTestVersions(c, a)

	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	sort.Strings(regDeleteCalls)
	c.Check(regDeleteCalls, check.DeepEquals, []string{
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v2",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v2-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v3",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v3-builder",
	})
	versions, err := servicemanager.AppVersion.AppVersions(a)
	c.Assert(err, check.IsNil)
	var appImgs, builderImgs []string
	var markedVersionsToRemoval []int
	for _, version := range versions.Versions {
		if version.MarkedToRemoval {
			markedVersionsToRemoval = append(markedVersionsToRemoval, version.Version)
			continue
		}
		if version.DeployImage != "" {
			appImgs = append(appImgs, version.DeployImage)
		}
		if version.BuildImage != "" {
			builderImgs = append(builderImgs, version.BuildImage)
		}
	}
	sort.Ints(markedVersionsToRemoval)
	sort.Strings(appImgs)
	sort.Strings(builderImgs)
	sort.Strings(nodeDeleteCalls)
	c.Check(markedVersionsToRemoval, check.DeepEquals, []int(nil))
	c.Check(appImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:v10",
		u.Host + "/tsuru/app-myapp:v11",
		u.Host + "/tsuru/app-myapp:v12",
		u.Host + "/tsuru/app-myapp:v13",
		u.Host + "/tsuru/app-myapp:v4",
		u.Host + "/tsuru/app-myapp:v5",
		u.Host + "/tsuru/app-myapp:v6",
		u.Host + "/tsuru/app-myapp:v7",
		u.Host + "/tsuru/app-myapp:v8",
		u.Host + "/tsuru/app-myapp:v9",
	})
	c.Check(builderImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:my-custom-tag",
		u.Host + "/tsuru/app-myapp:v10-builder",
		u.Host + "/tsuru/app-myapp:v11-builder",
		u.Host + "/tsuru/app-myapp:v12-builder",
		u.Host + "/tsuru/app-myapp:v13-builder",
		u.Host + "/tsuru/app-myapp:v4-builder",
		u.Host + "/tsuru/app-myapp:v5-builder",
		u.Host + "/tsuru/app-myapp:v6-builder",
		u.Host + "/tsuru/app-myapp:v7-builder",
		u.Host + "/tsuru/app-myapp:v8-builder",
		u.Host + "/tsuru/app-myapp:v9-builder",
	})
	c.Check(nodeDeleteCalls, check.DeepEquals, []string{
		"/images/" + u.Host + "/tsuru/app-myapp:my-custom-tag",
		"/images/" + u.Host + "/tsuru/app-myapp:v10",
		"/images/" + u.Host + "/tsuru/app-myapp:v10-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v11",
		"/images/" + u.Host + "/tsuru/app-myapp:v11-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v12",
		"/images/" + u.Host + "/tsuru/app-myapp:v12-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v2",
		"/images/" + u.Host + "/tsuru/app-myapp:v2-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v3",
		"/images/" + u.Host + "/tsuru/app-myapp:v3-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v4",
		"/images/" + u.Host + "/tsuru/app-myapp:v4-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v5",
		"/images/" + u.Host + "/tsuru/app-myapp:v5-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v6",
		"/images/" + u.Host + "/tsuru/app-myapp:v6-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v7",
		"/images/" + u.Host + "/tsuru/app-myapp:v7-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v8",
		"/images/" + u.Host + "/tsuru/app-myapp:v8-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v9",
		"/images/" + u.Host + "/tsuru/app-myapp:v9-builder",
	})
}

func (s *S) TestDryRunGCStartWithApp(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &app.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(a, s.user)
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

	config.Set("docker:registry", u.Host)
	config.Set("docker:gc:dry-run", true)
	defer config.Unset("docker:registry")
	defer config.Set("docker:gc:dry-run", false)
	insertTestVersions(c, a)

	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	// must never delete an image from registry when use dryRun
	c.Check(regDeleteCalls, check.DeepEquals, []string(nil))
	versions, err := servicemanager.AppVersion.AppVersions(a)
	c.Assert(err, check.IsNil)
	var appImgs, builderImgs []string
	var markedVersionsToRemoval []int
	for _, version := range versions.Versions {
		if version.MarkedToRemoval {
			markedVersionsToRemoval = append(markedVersionsToRemoval, version.Version)
			continue
		}
		if version.DeployImage != "" {
			appImgs = append(appImgs, version.DeployImage)
		}
		if version.BuildImage != "" {
			builderImgs = append(builderImgs, version.BuildImage)
		}
	}
	sort.Ints(markedVersionsToRemoval)
	sort.Strings(appImgs)
	sort.Strings(builderImgs)
	sort.Strings(nodeDeleteCalls)
	c.Check(markedVersionsToRemoval, check.DeepEquals, []int{2, 3})
	c.Check(appImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:v10",
		u.Host + "/tsuru/app-myapp:v11",
		u.Host + "/tsuru/app-myapp:v12",
		u.Host + "/tsuru/app-myapp:v13",
		u.Host + "/tsuru/app-myapp:v4",
		u.Host + "/tsuru/app-myapp:v5",
		u.Host + "/tsuru/app-myapp:v6",
		u.Host + "/tsuru/app-myapp:v7",
		u.Host + "/tsuru/app-myapp:v8",
		u.Host + "/tsuru/app-myapp:v9",
	})
	c.Check(builderImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:my-custom-tag",
		u.Host + "/tsuru/app-myapp:v10-builder",
		u.Host + "/tsuru/app-myapp:v11-builder",
		u.Host + "/tsuru/app-myapp:v12-builder",
		u.Host + "/tsuru/app-myapp:v13-builder",
		u.Host + "/tsuru/app-myapp:v4-builder",
		u.Host + "/tsuru/app-myapp:v5-builder",
		u.Host + "/tsuru/app-myapp:v6-builder",
		u.Host + "/tsuru/app-myapp:v7-builder",
		u.Host + "/tsuru/app-myapp:v8-builder",
		u.Host + "/tsuru/app-myapp:v9-builder",
	})
	c.Check(nodeDeleteCalls, check.DeepEquals, []string{
		"/images/" + u.Host + "/tsuru/app-myapp:my-custom-tag",
		"/images/" + u.Host + "/tsuru/app-myapp:v10",
		"/images/" + u.Host + "/tsuru/app-myapp:v10-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v11",
		"/images/" + u.Host + "/tsuru/app-myapp:v11-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v12",
		"/images/" + u.Host + "/tsuru/app-myapp:v12-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v2",
		"/images/" + u.Host + "/tsuru/app-myapp:v2-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v3",
		"/images/" + u.Host + "/tsuru/app-myapp:v3-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v4",
		"/images/" + u.Host + "/tsuru/app-myapp:v4-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v5",
		"/images/" + u.Host + "/tsuru/app-myapp:v5-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v6",
		"/images/" + u.Host + "/tsuru/app-myapp:v6-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v7",
		"/images/" + u.Host + "/tsuru/app-myapp:v7-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v8",
		"/images/" + u.Host + "/tsuru/app-myapp:v8-builder",
		"/images/" + u.Host + "/tsuru/app-myapp:v9",
		"/images/" + u.Host + "/tsuru/app-myapp:v9-builder",
	})

	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	sort.Slice(evts, func(i, j int) bool { return evts[i].Kind.Name < evts[j].Kind.Name })

	c.Check(evts[0].Target.Type, check.Equals, event.TargetTypeGC)
	c.Check(evts[0].Kind, check.Equals, event.Kind{Type: "internal", Name: "gc"})
	c.Check(evts[0].Error, check.Equals, "")

	c.Check(evts[1].Target.Type, check.Equals, event.TargetTypeApp)
	c.Check(evts[1].Target.Value, check.Equals, "myapp")
	c.Check(evts[1].Kind, check.Equals, event.Kind{Type: "internal", Name: "version gc"})
	c.Check(evts[1].Error, check.Equals, "")
}

func (s *S) TestGCStartWithAppStressNotFound(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &app.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(a, s.user)
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

	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	insertTestVersions(c, a)

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
	versions, err := servicemanager.AppVersion.AppVersions(a)
	c.Assert(err, check.IsNil)
	var appImgs, builderImgs []string
	var markedVersionsToRemoval []int
	for _, version := range versions.Versions {
		if version.MarkedToRemoval {
			markedVersionsToRemoval = append(markedVersionsToRemoval, version.Version)
			continue
		}
		if version.DeployImage != "" {
			appImgs = append(appImgs, version.DeployImage)
		}
		if version.BuildImage != "" {
			builderImgs = append(builderImgs, version.BuildImage)
		}
	}
	sort.Ints(markedVersionsToRemoval)
	sort.Strings(appImgs)
	sort.Strings(builderImgs)
	c.Check(markedVersionsToRemoval, check.DeepEquals, []int(nil))
	c.Check(appImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:v10",
		u.Host + "/tsuru/app-myapp:v11",
		u.Host + "/tsuru/app-myapp:v12",
		u.Host + "/tsuru/app-myapp:v13",
		u.Host + "/tsuru/app-myapp:v4",
		u.Host + "/tsuru/app-myapp:v5",
		u.Host + "/tsuru/app-myapp:v6",
		u.Host + "/tsuru/app-myapp:v7",
		u.Host + "/tsuru/app-myapp:v8",
		u.Host + "/tsuru/app-myapp:v9",
	})
	c.Check(builderImgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:my-custom-tag",
		u.Host + "/tsuru/app-myapp:v10-builder",
		u.Host + "/tsuru/app-myapp:v11-builder",
		u.Host + "/tsuru/app-myapp:v12-builder",
		u.Host + "/tsuru/app-myapp:v13-builder",
		u.Host + "/tsuru/app-myapp:v4-builder",
		u.Host + "/tsuru/app-myapp:v5-builder",
		u.Host + "/tsuru/app-myapp:v6-builder",
		u.Host + "/tsuru/app-myapp:v7-builder",
		u.Host + "/tsuru/app-myapp:v8-builder",
		u.Host + "/tsuru/app-myapp:v9-builder",
	})

	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	sort.Slice(evts, func(i, j int) bool { return evts[i].Kind.Name < evts[j].Kind.Name })

	c.Check(evts[0].Target.Type, check.Equals, event.TargetTypeGC)
	c.Check(evts[0].Kind, check.Equals, event.Kind{Type: "internal", Name: "gc"})
	c.Check(evts[0].Error, check.Equals, "")

	c.Check(evts[1].Target.Type, check.Equals, event.TargetTypeApp)
	c.Check(evts[1].Target.Value, check.Equals, "myapp")
	c.Check(evts[1].Kind, check.Equals, event.Kind{Type: "internal", Name: "version gc"})
	c.Check(evts[1].Error, check.Equals, "")
}

func (s *S) TestSelectAppVersions(c *check.C) {
	now := time.Now()
	testCases := []struct {
		explanation                            string
		historySize                            int
		appVersions                            func() appTypes.AppVersions
		deployedVersions                       []int
		expectedVersionsToRemove               []int
		expectedVersionsToPruneFromProvisioner []int
	}{
		{
			explanation: "should use ID to sort when is same updatedAt",
			historySize: 5,
			appVersions: func() appTypes.AppVersions {
				appVersions := appTypes.AppVersions{
					LastSuccessfulVersion: 10,
					Versions:              map[int]appTypes.AppVersionInfo{},
				}

				for i := 10; i > 0; i-- {
					appVersions.Versions[i] = appTypes.AppVersionInfo{
						Version:          i,
						DeploySuccessful: true,
						UpdatedAt:        now,
					}
				}

				return appVersions
			},
			expectedVersionsToRemove:               []int{5, 4, 3, 2, 1},
			expectedVersionsToPruneFromProvisioner: []int{9, 8, 7, 6},
		},
		{
			explanation: "should use updatedAt to short the versions",
			historySize: 5,
			appVersions: func() appTypes.AppVersions {
				appVersions := appTypes.AppVersions{
					LastSuccessfulVersion: 10,
					Versions:              map[int]appTypes.AppVersionInfo{},
				}

				for i := 10; i > 0; i-- {
					appVersions.Versions[i] = appTypes.AppVersionInfo{
						Version:          i,
						DeploySuccessful: true,
						UpdatedAt:        now.Add(time.Minute * time.Duration(i)),
					}
				}

				return appVersions
			},
			expectedVersionsToRemove:               []int{5, 4, 3, 2, 1},
			expectedVersionsToPruneFromProvisioner: []int{9, 8, 7, 6},
		},

		{
			explanation: "should always priorize unsucessful deployed versions to be removed",
			historySize: 10,
			appVersions: func() appTypes.AppVersions {
				appVersions := appTypes.AppVersions{
					LastSuccessfulVersion: 30,
					Versions:              map[int]appTypes.AppVersionInfo{},
				}

				for i := 30; i > 0; i-- {
					appVersions.Versions[i] = appTypes.AppVersionInfo{
						Version:          i,
						DeploySuccessful: (i % 2) == 0, // create a sampling with 50% failed deploys
						UpdatedAt:        now.Add(time.Minute * time.Duration(i)),
					}
				}

				return appVersions
			},
			// remove first all unsuccessful versions
			expectedVersionsToRemove: []int{29, 27, 25, 23, 21, 19, 17, 15, 13, 11, 9, 7, 5, 3, 1, 10, 8, 6, 4, 2},
			// clean all successful versions
			expectedVersionsToPruneFromProvisioner: []int{28, 26, 24, 22, 20, 18, 16, 14, 12},
		},

		{
			explanation:      "must never remove deployed versions",
			historySize:      10,
			deployedVersions: []int{20, 10, 8, 2},
			appVersions: func() appTypes.AppVersions {
				appVersions := appTypes.AppVersions{
					LastSuccessfulVersion: 20,
					Versions:              map[int]appTypes.AppVersionInfo{},
				}

				for i := 30; i > 0; i-- {
					appVersions.Versions[i] = appTypes.AppVersionInfo{
						Version:          i,
						DeploySuccessful: (i % 2) == 0, // create a sampling with 50% failed deploys
						UpdatedAt:        now.Add(time.Minute * time.Duration(i)),
					}
				}

				return appVersions
			},
			expectedVersionsToRemove:               []int{29, 27, 25, 23, 21, 19, 17, 15, 13, 11, 9, 7, 5, 3, 1, 6, 4},
			expectedVersionsToPruneFromProvisioner: []int{28, 26, 24, 22, 18, 16, 14, 12},
		},
	}

	for _, testCase := range testCases {
		c.Log("Running: " + testCase.explanation)
		versionsToRemove, versionsToPruneFromProvisioner := selectAppVersions(testCase.appVersions(), testCase.deployedVersions, testCase.historySize)

		c.Check(versionIDs(versionsToRemove), check.DeepEquals, testCase.expectedVersionsToRemove)
		c.Check(versionIDs(versionsToPruneFromProvisioner), check.DeepEquals, testCase.expectedVersionsToPruneFromProvisioner)
		c.Log("Finished: " + testCase.explanation)
	}
}

func versionIDs(versions []appTypes.AppVersionInfo) []int {
	ids := []int{}
	for _, version := range versions {
		ids = append(ids, version.Version)
	}
	return ids
}
