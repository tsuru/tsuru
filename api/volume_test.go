// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision/pool"
	authTypes "github.com/tsuru/tsuru/types/auth"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	"github.com/tsuru/tsuru/volume"
	"gopkg.in/check.v1"
)

func (s *S) TestVolumeList(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	config.Set("volume-plans:nfs:fake:capacity", "20Gi")
	config.Set("volume-plans:nfs:fake:access-modes", "ReadWriteMany")
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err := v1.Save()
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []volume.Volume
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []volume.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Plan: volume.VolumePlan{
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
	err := pool.AddPool(pool.AddPoolOptions{Name: "otherpool", Public: true})
	c.Assert(err, check.IsNil)
	err = serviceTypes.Team().Insert(authTypes.Team{Name: "otherteam"})
	c.Assert(err, check.IsNil)
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: "otherteam", Plan: volume.VolumePlan{Name: "nfs"}}
	err = v1.Save()
	c.Assert(err, check.IsNil)
	v2 := volume.Volume{Name: "v2", Pool: "otherpool", TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err = v2.Save()
	c.Assert(err, check.IsNil)
	token1 := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermVolumeRead,
		Context: permission.Context(permission.CtxPool, "otherpool"),
	})
	_, token2 := permissiontest.CustomUserWithPermission(c, nativeScheme, "majortom2", permission.Permission{
		Scheme:  permission.PermVolumeRead,
		Context: permission.Context(permission.CtxTeam, "otherteam"),
	})
	url := "/1.4/volumes"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+token1.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []volume.Volume
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].Name, check.Equals, "v2")
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
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	config.Set("volume-plans:nfs:fake:capacity", "20Gi")
	config.Set("volume-plans:nfs:fake:access-modes", "ReadWriteMany")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err = v1.Save()
	c.Assert(err, check.IsNil)
	err = v1.BindApp(a.Name, "/mnt", false)
	c.Assert(err, check.IsNil)
	err = v1.BindApp(a.Name, "/mnt2", true)
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result []volume.Volume
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []volume.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Binds: []volume.VolumeBind{
			{
				ID: volume.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt",
					Volume:     "v1",
				},
				ReadOnly: false,
			},
			{
				ID: volume.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt2",
					Volume:     "v1",
				},
				ReadOnly: true,
			},
		},
		Plan: volume.VolumePlan{
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
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err := v1.Save()
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes/v1"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result volume.Volume
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, volume.Volume{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Plan: volume.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	})
}

func (s *S) TestVolumeInfoNotFound(c *check.C) {
	url := "/1.4/volumes/v1"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestVolumeCreate(c *check.C) {
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
	volumes, err := volume.ListByFilter(nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, []volume.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Opts:      map[string]string{"a": "b"},
		Plan: volume.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	}})
}

func (s *S) TestVolumeCreateConflict(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err := v1.Save()
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
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}, Opts: map[string]string{"a": "b"}}
	err := v1.Save()
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
	volumes, err := volume.ListByFilter(nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, []volume.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Opts:      map[string]string{"a": "c"},
		Plan: volume.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	}})
}

func (s *S) TestVolumeUpdateNotFound(c *check.C) {
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
	defer config.Unset("volume-plans")
	url := "/1.4/volumeplans"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result map[string][]volume.VolumePlan
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	for key := range result {
		sort.Slice(result[key], func(i, j int) bool { return result[key][i].Name < result[key][j].Name })
	}
	c.Assert(result, check.DeepEquals, map[string][]volume.VolumePlan{
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
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err := v1.Save()
	c.Assert(err, check.IsNil)
	url := "/1.4/volumes/v1"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	volumes, err := volume.ListByFilter(nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.HasLen, 0)
}

func (s *S) TestVolumeBind(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err := v1.Save()
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
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
	volumes, err := volume.ListByFilter(nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, []volume.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Binds: []volume.VolumeBind{
			{
				ID: volume.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt1",
					Volume:     "v1",
				},
				ReadOnly: false,
			},
			{
				ID: volume.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt2",
					Volume:     "v1",
				},
				ReadOnly: true,
			},
		},
		Plan: volume.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	}})
}

func (s *S) TestVolumeBindConflict(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err := v1.Save()
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
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
	request, err = http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestVolumeBindNoRestart(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err := v1.Save()
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
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
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err = v1.Save()
	c.Assert(err, check.IsNil)
	err = v1.BindApp(a.Name, "/mnt1", false)
	c.Assert(err, check.IsNil)
	err = v1.BindApp(a.Name, "/mnt2", true)
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
	volumes, err := volume.ListByFilter(nil)
	c.Assert(err, check.IsNil)
	c.Assert(volumes, check.DeepEquals, []volume.Volume{{
		Name:      "v1",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Binds: []volume.VolumeBind{
			{
				ID: volume.VolumeBindID{
					App:        "myapp",
					MountPoint: "/mnt1",
					Volume:     "v1",
				},
				ReadOnly: false,
			},
		},
		Plan: volume.VolumePlan{
			Name: "nfs",
			Opts: map[string]interface{}{
				"plugin": "nfs",
			},
		},
	}})
}

func (s *S) TestVolumeUnbindNotFound(c *check.C) {
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err = v1.Save()
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
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	a := app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err = v1.Save()
	c.Assert(err, check.IsNil)
	err = v1.BindApp(a.Name, "/mnt1", false)
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
