// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestArchiveFile(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	bopts := builder.BuildOpts{
		ArchiveFile: ioutil.NopCloser(buf),
		ArchiveSize: int64(buf.Len()),
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img.BuildImageName(), check.Equals, "tsuru/app-myapp:v1-builder")
}

func (s *S) TestArchiveFileWithTag(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	a.TeamOwner = "admin"
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	bopts := builder.BuildOpts{
		ArchiveFile: ioutil.NopCloser(buf),
		ArchiveSize: int64(buf.Len()),
		Tag:         "mytag",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img.BuildImageName(), check.Equals, s.team.Name+"/app-myapp:mytag")
}

func (s *S) TestArchiveURL(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("my archive data"))
	}))
	defer ts.Close()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	bopts := builder.BuildOpts{
		ArchiveURL: ts.URL + "/myfile.tgz",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `build image from ArchiveURL is not yet supported by kubernetes builder`)
	c.Assert(img, check.IsNil)
}

func (s *S) TestImageID(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
			"image": {"Config": {"Cmd": ["arg1"], "Entrypoint": ["run", "mycmd"], "ExposedPorts": null}},
			"procfile": "web: make run",
			"tsuruYaml": {"healthcheck": {"path": "/health",  "scheme": "https"}}
		}`
		w.Write([]byte(output))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img.BaseImageName(), check.Equals, "tsuru/app-myapp:v1")
	c.Assert(img.IsBuild(), check.Equals, false)
}

func (s *S) TestImageIDWithExposedPorts(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
"image": {"Config": {"Cmd": ["arg1"], "Entrypoint": ["run", "mycmd"], "ExposedPorts": {"8000/tcp": {}, "8001/tcp": {}}}},
			"procfile": "web: make run",
			"tsuruYaml": {"healthcheck": {"path": "/health",  "scheme": "https"}}
		}`
		w.Write([]byte(output))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img.BaseImageName(), check.Equals, "tsuru/app-myapp:v1")
	c.Assert(img.IsBuild(), check.Equals, false)
	imd, err := image.GetImageMetaData(img.BaseImageName())
	c.Assert(err, check.IsNil)
	sort.Strings(imd.ExposedPorts)
	c.Assert(imd.ExposedPorts, check.DeepEquals, []string{"8000/tcp", "8001/tcp"})
}

func (s *S) TestImageIDWithProcfile(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
			"image": {"Config": {"Cmd": null, "Entrypoint": null, "ExposedPorts": null}},
			"procfile": "web: test.sh",
			"tsuruYaml": {"healthcheck": {"path": "/health",  "scheme": "https"}}
		}`
		w.Write([]byte(output))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img.BaseImageName(), check.Equals, "tsuru/app-myapp:v1")
	c.Assert(img.IsBuild(), check.Equals, false)
	imd, err := image.GetImageMetaData(img.BaseImageName())
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"test.sh"}}
	c.Assert(imd.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestImageIDWithTsuruYaml(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
			"image": {"Config": {"Cmd": null, "Entrypoint": null, "ExposedPorts": null}},
			"procfile": "web: test.sh",
			"tsuruYaml": {
				"healthcheck": {
					"path": "/status",
					"status": 200,
					"method":"GET",
					"scheme": "https"
				},
				"hooks": {
					"build": ["./build1", "./build2"],
					"restart": {
						"before": ["./before.sh"],
						"after": ["./after.sh"]
					}
				},
				"kubernetes": {
					"groups": {
						"pod1": {
							"web": {
								"ports": [
									{
										"name": "main-port",
										"target_port": 8000
									},
									{
										"port": 8080,
										"target_port": 8001
									}
								]
							}
						}
					}
				}
			}
		}`
		w.Write([]byte(output))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img.BaseImageName(), check.Equals, "tsuru/app-myapp:v1")
	c.Assert(img.IsBuild(), check.Equals, false)
	imd, err := image.GetImageMetaData(img.BaseImageName())
	c.Assert(err, check.IsNil)
	c.Assert(imd.CustomData, check.DeepEquals, map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/status",
			"method": "GET",
			"status": float64(200),
			"scheme": "https",
		},
		"hooks": map[string]interface{}{
			"build": []interface{}{"./build1", "./build2"},
			"restart": map[string]interface{}{
				"before": []interface{}{"./before.sh"},
				"after":  []interface{}{"./after.sh"},
			},
		},
		"kubernetes": map[string]interface{}{
			"groups": map[string]interface{}{
				"pod1": map[string]interface{}{
					"web": map[string]interface{}{
						"ports": []interface{}{
							map[string]interface{}{
								"name":        "main-port",
								"target_port": float64(8000),
							},
							map[string]interface{}{
								"port":        float64(8080),
								"target_port": float64(8001),
							},
						},
					},
				},
			},
		},
	})
}

func (s *S) TestImageIDWithTsuruYamlNoHealthcheck(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		output := `{
			"image": {"Config": {"Cmd": null, "Entrypoint": null, "ExposedPorts": null}},
			"procfile": "web: test.sh",
			"tsuruYaml": {
				"hooks": {
					"build": ["./build1", "./build2"],
					"restart": {
						"before": ["./before.sh"],
						"after": ["./after.sh"]
					}
				}
			}
		}`
		w.Write([]byte(output))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img.BaseImageName(), check.Equals, "tsuru/app-myapp:v1")
	c.Assert(img.IsBuild(), check.Equals, false)
	imd, err := image.GetImageMetaData(img.BaseImageName())
	c.Assert(err, check.IsNil)
	c.Assert(imd.CustomData, check.DeepEquals, map[string]interface{}{
		"hooks": map[string]interface{}{
			"build": []interface{}{"./build1", "./build2"},
			"restart": map[string]interface{}{
				"before": []interface{}{"./before.sh"},
				"after":  []interface{}{"./after.sh"},
			},
		},
	})
}

func (s *S) TestImageIDInspectError(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		w.Write([]byte(`x
ignored docker tag output
ignored docker push output
`))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	_, err = s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "invalid image inspect response: \"x\\nignored docker tag output\\nignored docker push output\\n\": invalid character 'x' looking for beginning of value")
}

func (s *S) TestRebuild(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	bopts := builder.BuildOpts{
		ArchiveFile: ioutil.NopCloser(buf),
		ArchiveSize: int64(buf.Len()),
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img.BuildImageName(), check.Equals, "tsuru/app-myapp:v1-builder")
	bopts = builder.BuildOpts{
		Rebuild: true,
	}
	img, err = s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	c.Assert(img.BuildImageName(), check.Equals, "tsuru/app-myapp:v2-builder")
}
