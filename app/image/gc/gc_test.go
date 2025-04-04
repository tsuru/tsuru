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
	"github.com/tsuru/tsuru/db/storagev2"
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
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	authTypes "github.com/tsuru/tsuru/types/auth"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	user        *auth.User
	team        string
	mockService servicemock.MockService
	cluster     *provisionTypes.Cluster
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	var err error
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "app_image_gc_tests")
	config.Set("docker:collection", "docker")
	config.Set("docker:repository-namespace", "tsuru")
	config.Set("routers:fake:type", "fake")
	config.Set("auth:hash-cost", bcrypt.MinCost)

	storagev2.Reset()

	provision.DefaultProvisioner = "fake"
	app.AuthScheme = auth.ManagedScheme(native.NativeScheme{})
	s.cluster = &provisionTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Provisioner: "fake",
		Default:     true,
	}
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	provisiontest.ProvisionerInstance.Reset()
	routertest.FakeRouter.Reset()
	s.user, _ = permissiontest.CustomUserWithPermission(c, app.AuthScheme, "majortom", permTypes.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	s.team = "myteam"
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name: "p1",
	})
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&s.mockService)
	plan := appTypes.Plan{
		Name:    "default",
		Default: true,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &plan, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == plan.Name {
			return &plan, nil
		}
		return nil, appTypes.ErrPlanNotFound
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
	s.mockService.Cluster.OnFindByName = func(name string) (*provisionTypes.Cluster, error) {
		c.Assert(name, check.Equals, s.cluster.Name)
		return nil, provisionTypes.ErrNoCluster
	}
	s.mockService.Cluster.OnList = func() ([]provisionTypes.Cluster, error) {
		return []provisionTypes.Cluster{*s.cluster}, nil
	}
	s.mockService.App.OnRegistry = func(app *appTypes.App) (imgTypes.ImageRegistry, error) {
		registry := s.cluster.CustomData["registry"]
		return imgTypes.ImageRegistry(registry), nil
	}
}

func (s *S) TearDownTest(c *check.C) {
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func insertTestVersions(c *check.C, a *appTypes.App, desiredNumberOfVersions int) {
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App:            a,
		EventID:        primitive.NewObjectID().Hex(),
		CustomBuildTag: "my-custom-tag",
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBuildImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	for i := 1; i <= desiredNumberOfVersions; i++ {
		version, err = servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
			App:     a,
			EventID: primitive.NewObjectID().Hex(),
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
		if r.Method == "HEAD" {
			w.Header().Set("Docker-Content-Digest", r.URL.Path)
			return
		}
		if r.Method == "DELETE" {
			regDeleteCalls = append(regDeleteCalls, r.URL.Path)
		}
	}))
	u, _ := url.Parse(srv.URL)
	s.cluster.CustomData = map[string]string{"registry": u.Host + "/tsuru"}
	defer srv.Close()
	fakeApp := provisiontest.NewFakeApp("myapp", "go", 0)
	insertTestVersions(c, fakeApp, 10)
	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err := gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	expectedCalls := []string{
		"/v2/tsuru/app-myapp/manifests/v2",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v2",
		"/v2/tsuru/app-myapp/manifests/v3",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v3",
		"/v2/tsuru/app-myapp/manifests/v4",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v4",
		"/v2/tsuru/app-myapp/manifests/v5",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v5",
		"/v2/tsuru/app-myapp/manifests/v6",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v6",
		"/v2/tsuru/app-myapp/manifests/v7",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v7",
		"/v2/tsuru/app-myapp/manifests/v8",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v8",
		"/v2/tsuru/app-myapp/manifests/v9",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v9",
		"/v2/tsuru/app-myapp/manifests/v10",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v10",
		"/v2/tsuru/app-myapp/manifests/v11",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v11",
		"/v2/tsuru/app-myapp/manifests/my-custom-tag",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/my-custom-tag",
		"/v2/tsuru/app-myapp/manifests/v2-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v2-builder",
		"/v2/tsuru/app-myapp/manifests/v3-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v3-builder",
		"/v2/tsuru/app-myapp/manifests/v4-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v4-builder",
		"/v2/tsuru/app-myapp/manifests/v5-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v5-builder",
		"/v2/tsuru/app-myapp/manifests/v6-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v6-builder",
		"/v2/tsuru/app-myapp/manifests/v7-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v7-builder",
		"/v2/tsuru/app-myapp/manifests/v8-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v8-builder",
		"/v2/tsuru/app-myapp/manifests/v9-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v9-builder",
		"/v2/tsuru/app-myapp/manifests/v10-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v10-builder",
		"/v2/tsuru/app-myapp/manifests/v11-builder",
		"/v2/tsuru/app-myapp/manifests//v2/tsuru/app-myapp/manifests/v11-builder",
	}
	sort.Strings(regDeleteCalls)
	sort.Strings(expectedCalls)
	c.Assert(regDeleteCalls, check.DeepEquals, expectedCalls)
	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(len(versions.Versions), check.Equals, 0)
}

func (s *S) TestGCStartWithApp(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(context.TODO(), a, s.user)
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
	s.cluster.CustomData = map[string]string{"registry": u.Host + "/tsuru"}
	defer registrySrv.Close()

	insertTestVersions(c, a, 12)

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
		"/v2/tsuru/app-myapp/manifests/v2",
		"/v2/tsuru/app-myapp/manifests/v2-builder",
		"/v2/tsuru/app-myapp/manifests/v3",
		"/v2/tsuru/app-myapp/manifests/v3-builder",
	})
	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), a)
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
}

func (s *S) TestGCStartWithRunningEvent(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	nodeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer nodeSrv.Close()
	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Docker-Content-Digest", r.URL.Path)
			return
		}
	}))
	u, _ := url.Parse(registrySrv.URL)
	s.cluster.CustomData = map[string]string{"registry": u.Host + "/tsuru"}
	defer registrySrv.Close()

	config.Set("docker:gc:dry-run", true)
	defer config.Set("docker:gc:dry-run", false)

	now := time.Now()

	for i := 0; i < 2; i++ {
		evt := event.Event{}
		evt.UniqueID = primitive.NewObjectID()
		evt.ID = evt.UniqueID
		if i%2 == 0 {
			evt.Running = true
		} else {
			evt.EndTime = now
		}

		err = evt.RawInsert(context.TODO(), nil, nil, nil)
		c.Assert(err, check.IsNil)

		var appVersion appTypes.AppVersion
		appVersion, err = servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
			App:     a,
			EventID: evt.UniqueID.Hex(),
		})

		c.Assert(err, check.IsNil)
		err = appVersion.CommitBuildImage()
		c.Assert(err, check.IsNil)
		err = appVersion.CommitBaseImage()
		c.Assert(err, check.IsNil)
	}

	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), a)
	c.Assert(err, check.IsNil)
	var imgs []string
	var markedVersionsToRemoval []int
	for _, version := range versions.Versions {
		if version.MarkedToRemoval {
			markedVersionsToRemoval = append(markedVersionsToRemoval, version.Version)
			continue
		}
		if version.DeployImage != "" {
			imgs = append(imgs, version.DeployImage)
		}
		if version.BuildImage != "" {
			imgs = append(imgs, version.BuildImage)
		}
	}
	sort.Ints(markedVersionsToRemoval)
	// the most important assert, we must never mark a version with running event related
	c.Assert(markedVersionsToRemoval, check.DeepEquals, []int{2})
	c.Assert(imgs, check.DeepEquals, []string{
		u.Host + "/tsuru/app-myapp:v1",
		u.Host + "/tsuru/app-myapp:v1-builder",
	})
}

func (s *S) TestGCStartIgnoreErrorOnProvisioner(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	nodeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unavailable", http.StatusInternalServerError)
	}))
	defer nodeSrv.Close()
	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Docker-Content-Digest", r.URL.Path)
		}
	}))
	u, _ := url.Parse(registrySrv.URL)
	s.cluster.CustomData = map[string]string{"registry": u.Host + "/tsuru"}
	defer registrySrv.Close()

	insertTestVersions(c, a, 11)

	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)

	evts, err := event.All(context.TODO())
	c.Assert(err, check.IsNil)
	evts = filterGCEvents(evts)
	c.Assert(evts, check.HasLen, 0)

	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Check(len(versions.Versions), check.Equals, 11)
}

func (s *S) TestGCStartWithErrorOnRegistry(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	nodeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer nodeSrv.Close()
	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unavailable", http.StatusInternalServerError)
	}))
	u, _ := url.Parse(registrySrv.URL)
	s.cluster.CustomData = map[string]string{"registry": u.Host + "/tsuru"}
	defer registrySrv.Close()

	insertTestVersions(c, a, 11)

	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)

	evts, err := event.All(context.TODO())
	c.Assert(err, check.IsNil)
	evts = filterGCEvents(evts)
	c.Assert(evts, check.HasLen, 1)
	if !c.Check(strings.Contains(evts[0].Error, "invalid status reading manifest for tsuru/app-myapp:v2: 500"), check.Equals, true) {
		fmt.Println(evts[0].Error)
	}

	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Check(len(versions.Versions), check.Equals, 12)
}

func (s *S) TestDryRunGCStartWithApp(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(context.TODO(), a, s.user)
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
	s.cluster.CustomData = map[string]string{"registry": u.Host + "/tsuru"}
	defer registrySrv.Close()

	config.Set("docker:gc:dry-run", true)
	defer config.Set("docker:gc:dry-run", false)
	insertTestVersions(c, a, 12)

	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	// must never delete an image from registry when use dryRun
	c.Check(regDeleteCalls, check.DeepEquals, []string(nil))
	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), a)
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
	evts, err := event.All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Check(evts[0].Target.Type, check.Equals, eventTypes.TargetTypeApp)
	c.Check(evts[0].Target.Value, check.Equals, "myapp")
	c.Check(evts[0].Kind, check.Equals, eventTypes.Kind{Type: "internal", Name: "version gc"})
	c.Check(evts[0].Error, check.Equals, "")
}

func (s *S) TestGCNoOPWithApp(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	var regDeleteCalls int
	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		regDeleteCalls++
	}))
	u, _ := url.Parse(registrySrv.URL)
	s.cluster.CustomData = map[string]string{"registry": u.Host + "/tsuru"}
	defer registrySrv.Close()

	insertTestVersions(c, a, 5)

	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	err = gc.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	c.Check(regDeleteCalls, check.Equals, 0)
	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Check(versions.Versions, check.HasLen, 6)

	evts, err := event.All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestGCStartWithAppStressNotFound(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team}}, nil
	}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team, Pool: "p1"}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	nodeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer nodeSrv.Close()
	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	u, _ := url.Parse(registrySrv.URL)
	s.cluster.CustomData = map[string]string{"registry": u.Host + "/tsuru"}
	defer registrySrv.Close()

	insertTestVersions(c, a, 12)

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
	versions, err := servicemanager.AppVersion.AppVersions(context.TODO(), a)
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

	evts, err := event.All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)

	c.Check(evts[0].Target.Type, check.Equals, eventTypes.TargetTypeApp)
	c.Check(evts[0].Target.Value, check.Equals, "myapp")
	c.Check(evts[0].Kind, check.Equals, eventTypes.Kind{Type: "internal", Name: "version gc"})
	c.Check(evts[0].Error, check.Equals, "")
}

func (s *S) TestSelectAppVersions(c *check.C) {
	now := time.Now()
	testCases := []struct {
		explanation                     string
		historySize                     int
		appVersions                     func() appTypes.AppVersions
		deployedVersions                []int
		expectedVersionsToRemove        []int
		expectedUnsuccessfulDeployments []int
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
			expectedVersionsToRemove:        []int{5, 4, 3, 2, 1},
			expectedUnsuccessfulDeployments: []int{},
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
			expectedVersionsToRemove:        []int{5, 4, 3, 2, 1},
			expectedUnsuccessfulDeployments: []int{},
		},

		{
			explanation: "should return unsuccessful deployed versions in the specified array",
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
			expectedVersionsToRemove:        []int{10, 8, 6, 4, 2},
			expectedUnsuccessfulDeployments: []int{29, 27, 25, 23, 21, 19, 17, 15, 13, 11, 9, 7, 5, 3, 1},
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
			expectedVersionsToRemove:        []int{6, 4},
			expectedUnsuccessfulDeployments: []int{29, 27, 25, 23, 21, 19, 17, 15, 13, 11, 9, 7, 5, 3, 1},
		},

		{
			explanation: "must never remove versions generated by app-build",
			appVersions: func() appTypes.AppVersions {
				appVersions := appTypes.AppVersions{
					LastSuccessfulVersion: 20,
					Versions: map[int]appTypes.AppVersionInfo{
						100: {
							Version:          100,
							DeploySuccessful: false,
							BuildImage:       "docker.com/app:myTag",
							CustomBuildTag:   "myTag",
						},
					},
				}

				return appVersions
			},
			expectedVersionsToRemove:        []int{},
			expectedUnsuccessfulDeployments: []int{},
		},
	}

	for _, testCase := range testCases {
		c.Log("Running: " + testCase.explanation)
		selection := selectAppVersions(testCase.appVersions(), testCase.deployedVersions, testCase.historySize)

		c.Check(versionIDs(selection.toRemove), check.DeepEquals, testCase.expectedVersionsToRemove)
		c.Check(versionIDs(selection.unsuccessfulDeploys), check.DeepEquals, testCase.expectedUnsuccessfulDeployments)
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

func filterGCEvents(evts []*event.Event) []*event.Event {
	n := 0
	for _, evt := range evts {
		if evt.Target.Type == eventTypes.TargetTypeGC {
			evts[n] = evt
			n++
		}
	}
	evts = evts[:n]
	return evts
}
