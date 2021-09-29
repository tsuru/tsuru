// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/kr/pretty"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	provTypes "github.com/tsuru/tsuru/types/provision"
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
	img, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBuildImage, err := img.BuildImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBuildImage, check.Equals, "tsuru/app-myapp:v1-builder")
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
	img, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBuildImage, err := img.BuildImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBuildImage, check.Equals, s.team.Name+"/app-myapp:mytag")
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
	img, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
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
	version, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, "tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, "")
	c.Assert(version.VersionInfo().DeployImage, check.Equals, testBaseImage)
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
	version, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, "tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, "")
	c.Assert(version.VersionInfo().DeployImage, check.Equals, testBaseImage)
	sort.Strings(version.VersionInfo().ExposedPorts)
	c.Assert(version.VersionInfo().ExposedPorts, check.DeepEquals, []string{"8000/tcp", "8001/tcp"})
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
	version, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, "tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, "")
	c.Assert(version.VersionInfo().DeployImage, check.Equals, testBaseImage)
	processes, err := version.Processes()
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"test.sh"}}
	c.Assert(processes, check.DeepEquals, expectedProcesses)
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
	version, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, "tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, "")
	c.Assert(version.VersionInfo().DeployImage, check.Equals, testBaseImage)

	customdata, err := version.TsuruYamlData()
	c.Assert(err, check.IsNil)
	expected := provTypes.TsuruYamlData{
		Healthcheck: &provTypes.TsuruYamlHealthcheck{
			Path:   "/status",
			Method: "GET",
			Status: 200,
			Scheme: "https",
		},
		Hooks: &provTypes.TsuruYamlHooks{
			Build: []string{"./build1", "./build2"},
			Restart: provTypes.TsuruYamlRestartHooks{
				Before: []string{"./before.sh"},
				After:  []string{"./after.sh"},
			},
		},
		Kubernetes: &provTypes.TsuruYamlKubernetesConfig{
			Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
				"pod1": {
					"web": provTypes.TsuruYamlKubernetesProcessConfig{
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{
								Name:       "main-port",
								TargetPort: 8000,
							},
							{
								Port:       8080,
								TargetPort: 8001,
							},
						},
					},
				},
			},
		},
	}
	c.Assert(customdata, check.DeepEquals, expected, check.Commentf("diff: %s", strings.Join(pretty.Diff(customdata, expected), "\n")))
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
	version, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, "tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, "")
	c.Assert(version.VersionInfo().DeployImage, check.Equals, testBaseImage)
	customdata, err := version.TsuruYamlData()
	c.Assert(err, check.IsNil)
	c.Assert(customdata, check.DeepEquals, provTypes.TsuruYamlData{
		Hooks: &provTypes.TsuruYamlHooks{
			Build: []string{"./build1", "./build2"},
			Restart: provTypes.TsuruYamlRestartHooks{
				Before: []string{"./before.sh"},
				After:  []string{"./after.sh"},
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
	_, err = s.b.Build(context.TODO(), s.p, a, evt, &bopts)
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
	version, err := s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBuildImage, err := version.BuildImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBuildImage, check.Equals, "tsuru/app-myapp:v1-builder")
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)

	bopts = builder.BuildOpts{
		Rebuild: true,
	}
	version, err = s.b.Build(context.TODO(), s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBuildImage2, err := version.BuildImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBuildImage2, check.Equals, "tsuru/app-myapp:v2-builder")
}
