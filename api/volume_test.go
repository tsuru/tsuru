// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	check "gopkg.in/check.v1"
)

func (s *S) TestVolumeList(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	config.Set("volume-plans:nfs:fake:capacity", "20Gi")
	config.Set("volume-plans:nfs:fake:access-modes", "ReadWriteMany")
	defer config.Unset("volume-plans")
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	s.mockService.VolumeService.OnListByFilter = func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
		return []volumeTypes.Volume{{
			Name:      "v1",
			Pool:      s.Pool,
			TeamOwner: s.team.Name,
			Plan: volumeTypes.VolumePlan{
				Name: "nfs",
				Opts: map[string]interface{}{
					"plugin":       "nfs",
					"capacity":     "20Gi",
					"access-modes": "ReadWriteMany",
				},
			},
		}}, nil
	}
	err := servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []volumeTypes.Volume
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []volumeTypes.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Plan: volumeTypes.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin":       "nfs",
				"capacity":     "20Gi",
				"access-modes": "ReadWriteMany",
			},
		},
	}})
}

func (s *S) TestVolumeListPermissions(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "otherpool", Public: true})
	c.Assert(err, check.IsNil)
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: "otherteam", Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	v2 := volumeTypes.Volume{Name: "v2", Pool: "otherpool", TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v2)
	c.Assert(err, check.IsNil)
	token1 := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermVolumeRead,
		Context: permission.Context(permTypes.CtxPool, "otherpool"),
	})
	_, token2 := permissiontest.CustomUserWithPermission(c, nativeScheme, "majortom2", permission.Permission{
		Scheme:  permission.PermVolumeRead,
		Context: permission.Context(permTypes.CtxTeam, "otherteam"),
	})
	s.mockService.VolumeService.OnListByFilter = func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
		return []volumeTypes.Volume{v2}, nil
	}
	url := "/1.4/volumes"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+token1.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []volumeTypes.Volume
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].Name, check.Equals, "v2")
	s.mockService.VolumeService.OnListByFilter = func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
		return []volumeTypes.Volume{v1}, nil
	}
	request, err = http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+token2.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].Name, check.Equals, "v1")
}

func (s *S) TestVolumeListBinded(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	config.Set("volume-plans:nfs:fake:capacity", "20Gi")
	config.Set("volume-plans:nfs:fake:access-modes", "ReadWriteMany")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)
	s.mockService.VolumeService.OnListByFilter = func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
		return []volumeTypes.Volume{{
			Name:      "v1",
			Pool:      s.Pool,
			TeamOwner: s.team.Name,
			Binds: []volumeTypes.VolumeBind{
				{
					ID: volumeTypes.VolumeBindID{
						App:        "myapp",
						MountPoint: "/mnt",
						Volume:     "v1",
					},
					ReadOnly: false,
				},
				{
					ID: volumeTypes.VolumeBindID{
						App:        "myapp",
						MountPoint: "/mnt2",
						Volume:     "v1",
					},
					ReadOnly: true,
				},
			},
			Plan: volumeTypes.VolumePlan{
				Name: "nfs",
				Opts: map[string]interface{}{
					"plugin":       "nfs",
					"capacity":     "20Gi",
					"access-modes": "ReadWriteMany",
				},
			},
		}}, nil
	}
	url := "/1.4/volumes"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []volumeTypes.Volume
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []volumeTypes.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Binds: []volumeTypes.VolumeBind{
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt",
					Volume:     "v1",
				},
				ReadOnly: false,
			},
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt2",
					Volume:     "v1",
				},
				ReadOnly: true,
			},
		},
		Plan: volumeTypes.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin":       "nfs",
				"capacity":     "20Gi",
				"access-modes": "ReadWriteMany",
			},
		},
	}})
}

func (s *S) TestVolumeInfo(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	config.Set("volume-plans:nfs:fake:capacity", "20Gi")
	config.Set("volume-plans:nfs:fake:access-modes", "ReadWriteMany")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		v1.Plan = volumeTypes.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"access-modes": "ReadWriteMany",
				"capacity":     "20Gi",
				"plugin":       "nfs",
			},
		}
		return &v1, nil
	}
	s.mockService.VolumeService.OnBinds = func(ctx context.Context, v *volumeTypes.Volume) ([]volumeTypes.VolumeBind, error) {
		return []volumeTypes.VolumeBind{
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt",
					Volume:     "v1",
				},
				ReadOnly: false,
			},
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt2",
					Volume:     "v1",
				},
				ReadOnly: true,
			},
		}, nil
	}
	url := "/1.4/volumes/v1"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result volumeTypes.Volume
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, volumeTypes.Volume{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Plan: volumeTypes.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"access-modes": "ReadWriteMany",
				"capacity":     "20Gi",
				"plugin":       "nfs",
			},
		},
		Binds: []volumeTypes.VolumeBind{
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt",
					Volume:     "v1",
				},
				ReadOnly: false,
			},
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt2",
					Volume:     "v1",
				},
				ReadOnly: true,
			},
		},
	})
}

func (s *S) TestVolumeInfoNotFound(c *check.C) {
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return nil, volumeTypes.ErrVolumeNotFound
	}
	url := "/1.4/volumes/v1"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestVolumeCreate(c *check.C) {
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "test1",
		Field:    pool.ConstraintTypeVolumePlan,
		Values:   []string{"nfs"},
	})
	c.Assert(err, check.IsNil)
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return nil, volumeTypes.ErrVolumeNotFound
	}
	s.mockService.VolumeService.OnListByFilter = func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
		return []volumeTypes.Volume{
			{
				Name:      "v1",
				Pool:      s.Pool,
				TeamOwner: s.team.Name,
				Opts:      map[string]string{"a": "b"},
				Plan: volumeTypes.VolumePlan{
					Name: "nfs",
					Opts: map[string]interface{}{
						"plugin": "nfs",
					},
				},
			},
		}, nil
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	body := strings.NewReader(`name=v1&pool=test1&teamowner=tsuruteam&plan.name=nfs&status=ignored&plan.opts.something=ignored&opts.a=b`)
	url := "/1.4/volumes"
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	volumes, err := servicemanager.Volume.ListByFilter(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, []volumeTypes.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Opts:      map[string]string{"a": "b"},
		Plan: volumeTypes.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	}})
}

func (s *S) TestVolumeCreateConflict(c *check.C) {
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "test1",
		Field:    pool.ConstraintTypeVolumePlan,
		Values:   []string{"nfs"},
	})
	c.Assert(err, check.IsNil)
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`name=v1&pool=test1&teamowner=tsuruteam&plan.name=nfs&status=ignored&plan.opts.something=ignored`)
	url := "/1.4/volumes"
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestVolumeUpdate(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}, Opts: map[string]string{"a": "b"}}
	err := servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`name=v1&pool=test1&teamowner=tsuruteam&plan.name=nfs&status=ignored&plan.opts.something=ignored&opts.a=c`)
	url := "/1.4/volumes/v1"
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	s.mockService.VolumeService.OnListByFilter = func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
		return []volumeTypes.Volume{
			{
				Name:      "v1",
				Pool:      s.Pool,
				TeamOwner: s.team.Name,
				Opts:      map[string]string{"a": "c"},
				Plan: volumeTypes.VolumePlan{
					Name: "nfs",
					Opts: map[string]interface{}{
						"plugin": "nfs",
					},
				},
			},
		}, nil
	}
	volumes, err := servicemanager.Volume.ListByFilter(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, []volumeTypes.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Opts:      map[string]string{"a": "c"},
		Plan: volumeTypes.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	}})
}

func (s *S) TestVolumeUpdateNotFound(c *check.C) {
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "test1",
		Field:    pool.ConstraintTypeVolumePlan,
		Values:   []string{"nfs"},
	})
	c.Assert(err, check.IsNil)
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return nil, volumeTypes.ErrVolumeNotFound
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	body := strings.NewReader(`name=v1&pool=test1&teamowner=tsuruteam&plan.name=nfs&status=ignored&plan.opts.something=ignored&opts.a=c`)
	url := "/1.4/volumes/v1"
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestVolumePlanList(c *check.C) {
	config.Set("volume-plans:nfs1:fake:plugin", "nfs")
	config.Set("volume-plans:other:fake:storage-class", "ebs")
	config.Set("volume-plans:nfs1:otherprov:opts", "-t=nfs")
	s.mockService.VolumeService.OnListPlans = func(ctx context.Context) (map[string][]volumeTypes.VolumePlan, error) {
		return map[string][]volumeTypes.VolumePlan{
			"fake": {
				{Name: "nfs1", Opts: map[string]interface{}{"plugin": "nfs"}},
				{Name: "other", Opts: map[string]interface{}{"storage-class": "ebs"}},
			},
			"otherprov": {
				{Name: "nfs1", Opts: map[string]interface{}{"opts": "-t=nfs"}},
			},
		}, nil
	}
	defer config.Unset("volume-plans")
	url := "/1.4/volumeplans"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result map[string][]volumeTypes.VolumePlan
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	for key := range result {
		sort.Slice(result[key], func(i, j int) bool { return result[key][i].Name < result[key][j].Name })
	}
	c.Assert(result, check.DeepEquals, map[string][]volumeTypes.VolumePlan{
		"fake": {
			{Name: "nfs1", Opts: map[string]interface{}{"plugin": "nfs"}},
			{Name: "other", Opts: map[string]interface{}{"storage-class": "ebs"}},
		},
		"otherprov": {
			{Name: "nfs1", Opts: map[string]interface{}{"opts": "-t=nfs"}},
		},
	})
}

func (s *S) TestVolumeDelete(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err := servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes/v1"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	volumes, err := servicemanager.Volume.ListByFilter(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.HasLen, 0)
}

func (s *S) TestVolumeBind(c *check.C) {
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return &v1, nil
	}
	s.mockService.VolumeService.OnListByFilter = func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
		return []volumeTypes.Volume{{
			Name:      "v1",
			Pool:      s.Pool,
			TeamOwner: s.team.Name,
			Binds: []volumeTypes.VolumeBind{
				{
					ID: volumeTypes.VolumeBindID{
						App:        "myapp",
						MountPoint: "/mnt1",
						Volume:     "v1",
					},
					ReadOnly: false,
				},
				{
					ID: volumeTypes.VolumeBindID{
						App:        "myapp",
						MountPoint: "/mnt2",
						Volume:     "v1",
					},
					ReadOnly: true,
				},
			},
			Plan: volumeTypes.VolumePlan{
				Name: "nfs",
				Opts: map[string]interface{}{
					"plugin": "nfs",
				},
			},
		}}, nil
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	url := "/1.4/volumes/v1/bind"
	body := strings.NewReader(`app=myapp&mountpoint=/mnt1&readonly=false`)
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body = strings.NewReader(`app=myapp&mountpoint=/mnt2&readonly=true`)
	request, err = http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches, `(?s).*restarting app.*`)
	volumes, err := servicemanager.Volume.ListByFilter(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, []volumeTypes.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Binds: []volumeTypes.VolumeBind{
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt1",
					Volume:     "v1",
				},
				ReadOnly: false,
			},
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt2",
					Volume:     "v1",
				},
				ReadOnly: true,
			},
		},
		Plan: volumeTypes.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	}})
}

func (s *S) TestVolumeBindConflict(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return &v1, nil
	}
	err := servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes/v1/bind"
	body := strings.NewReader(`app=myapp&mountpoint=/mnt1&readonly=false`)
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body = strings.NewReader(`app=myapp&mountpoint=/mnt1&readonly=true`)
	s.mockService.VolumeService.OnBindApp = func(ctx context.Context, opts *volumeTypes.BindOpts) error {
		return &errors.HTTP{Code: http.StatusConflict}
	}
	request, err = http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestVolumeBindNoRestart(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return &v1, nil
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes/v1/bind"
	body := strings.NewReader(`app=myapp&mountpoint=/mnt1&readonly=false&norestart=true`)
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "")
}

func (s *S) TestVolumeUnbind(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return &v1, nil
	}
	s.mockService.VolumeService.OnListByFilter = func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
		return []volumeTypes.Volume{{
			Name:      "v1",
			Pool:      s.Pool,
			TeamOwner: s.team.Name,
			Binds: []volumeTypes.VolumeBind{
				{
					ID: volumeTypes.VolumeBindID{
						App:        "myapp",
						MountPoint: "/mnt1",
						Volume:     "v1",
					},
					ReadOnly: false,
				},
			},
			Plan: volumeTypes.VolumePlan{
				Name: "nfs",
				Opts: map[string]interface{}{
					"plugin": "nfs",
				},
			},
		}}, nil
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt1",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes/v1/bind?app=myapp&mountpoint=/mnt2"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches, `(?s).*restarting app.*`)
	volumes, err := servicemanager.Volume.ListByFilter(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, []volumeTypes.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Binds: []volumeTypes.VolumeBind{
			{
				ID: volumeTypes.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt1",
					Volume:     "v1",
				},
				ReadOnly: false,
			},
		},
		Plan: volumeTypes.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	}})
}

func (s *S) TestVolumeUnbindNotFound(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return nil, volumeTypes.ErrVolumeNotFound
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes/v1/bind?app=myapp&mountpoint=/mnt1"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestVolumeUnbindNoRestart(c *check.C) {
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	s.mockService.VolumeService.OnGet = func(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
		return &v1, nil
	}
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt1",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes/v1/bind?app=myapp&mountpoint=/mnt1&norestart=true"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "")
}
