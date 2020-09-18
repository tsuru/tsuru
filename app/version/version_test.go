// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"context"

	"github.com/tsuru/config"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestAppVersionImpl_ImageNames(c *check.C) {
	tests := []struct {
		spec        appVersionImpl
		wantedBase  string
		wantedBuild string
		registry    string
		emptyRepoNS bool
	}{
		{
			spec:        appVersionImpl{app: &appTypes.MockApp{Name: "myapp"}, versionInfo: &appTypes.AppVersionInfo{Version: 1}},
			wantedBase:  "tsuru/app-myapp:v1",
			wantedBuild: "tsuru/app-myapp:v1-builder",
		},
		{
			spec:        appVersionImpl{app: &appTypes.MockApp{Name: "myapp"}, versionInfo: &appTypes.AppVersionInfo{Version: 2}},
			registry:    "localhost:3030",
			wantedBase:  "localhost:3030/tsuru/app-myapp:v2",
			wantedBuild: "localhost:3030/tsuru/app-myapp:v2-builder",
		},
		{
			spec:        appVersionImpl{app: &appTypes.MockApp{Name: "myapp", TeamOwner: "myteam"}, versionInfo: &appTypes.AppVersionInfo{Version: 9}},
			wantedBase:  "tsuru/app-myapp:v9",
			wantedBuild: "tsuru/app-myapp:v9-builder",
		},
		{
			spec:        appVersionImpl{app: &appTypes.MockApp{Name: "myapp", TeamOwner: "myteam"}, versionInfo: &appTypes.AppVersionInfo{Version: 9, CustomBuildTag: "mytag"}},
			wantedBase:  "tsuru/app-myapp:v9",
			wantedBuild: "tsuru/app-myapp:mytag",
		},
		{
			spec:        appVersionImpl{app: &appTypes.MockApp{Name: "myapp", TeamOwner: "myteam"}, versionInfo: &appTypes.AppVersionInfo{Version: 9}},
			emptyRepoNS: true,
			wantedBase:  "tsuru/app-myapp:v9",
			wantedBuild: "myteam/app-myapp:v9-builder",
		},
	}

	for i, tt := range tests {
		func() {
			c.Log("test", i)
			if tt.registry != "" {
				config.Set("docker:registry", tt.registry)
				defer config.Unset("docker:registry")
			}
			if tt.emptyRepoNS {
				config.Unset("docker:repository-namespace")
				defer config.Set("docker:repository-namespace", "tsuru")
			}
			c.Check(tt.spec.BaseImageName(), check.Equals, tt.wantedBase)
			c.Check(tt.spec.BuildImageName(), check.Equals, tt.wantedBuild)
		}()
	}
}

func (s *S) TestAppVersionImpl_AddData(c *check.C) {
	tests := []struct {
		name              string
		addData           appTypes.AddVersionDataArgs
		expectedYamlData  provTypes.TsuruYamlData
		expectedProcesses map[string][]string
		expectedPorts     []string
		expectedRaw       map[string]interface{}
	}{
		{
			name:              "empty is okay",
			expectedProcesses: map[string][]string{},
			expectedPorts:     []string{},
		},
		{
			name: "only exposed ports",
			addData: appTypes.AddVersionDataArgs{
				ExposedPorts: []string{"80/tcp", "22/tcp"},
			},
			expectedProcesses: map[string][]string{},
			expectedPorts:     []string{"80/tcp", "22/tcp"},
		},
		{
			name: "explicit processes",
			addData: appTypes.AddVersionDataArgs{
				Processes: map[string][]string{
					"worker1": {"python myapp.py"},
				},
			},
			expectedProcesses: map[string][]string{
				"worker1": {"python myapp.py"},
			},
			expectedPorts: []string{},
		},
		{
			name: "processes priority over procfile",
			addData: appTypes.AddVersionDataArgs{
				CustomData: map[string]interface{}{
					"procfile": "worker9: ignored",
					"processes": map[string]interface{}{
						"worker1": "python myapp.py",
						"worker2": "someworker",
					},
				},
			},
			expectedProcesses: map[string][]string{
				"worker1": {"python myapp.py"},
				"worker2": {"someworker"},
			},
			expectedPorts: []string{},
		},
		{
			name: "procfile used as fallback",
			addData: appTypes.AddVersionDataArgs{
				CustomData: map[string]interface{}{
					"procfile": "worker1: python myapp.py\nworker2: someworker",
				},
			},
			expectedProcesses: map[string][]string{
				"worker1": {"python myapp.py"},
				"worker2": {"someworker"},
			},
			expectedPorts: []string{},
		},
		{
			name: "processes with mixed list and string",
			addData: appTypes.AddVersionDataArgs{
				CustomData: map[string]interface{}{
					"processes": map[string]interface{}{
						"worker1": "python myapp.py",
						"worker2": []string{"worker", "arg", "arg2"},
					},
				},
			},
			expectedProcesses: map[string][]string{
				"worker1": {"python myapp.py"},
				"worker2": {"worker", "arg", "arg2"},
			},
			expectedPorts: []string{},
		},
		{
			name: "saves unknown fields for future use",
			addData: appTypes.AddVersionDataArgs{
				CustomData: map[string]interface{}{
					"myfield": "myvalue",
				},
			},
			expectedRaw: map[string]interface{}{
				"myfield":     "myvalue",
				"healthcheck": nil,
				"hooks":       nil,
			},
			expectedProcesses: map[string][]string{},
			expectedPorts:     []string{},
		},
		{
			name: "parse and recover hooks and complex kubernetes",
			addData: appTypes.AddVersionDataArgs{
				CustomData: map[string]interface{}{
					"hooks": map[string]interface{}{
						"build": []string{"script1", "script2"},
					},
					"healthcheck": map[string]interface{}{
						"path": "/status",
					},
					"kubernetes": map[string]interface{}{
						"groups": map[string]interface{}{
							"pod1": map[string]interface{}{
								"proc1": map[string]interface{}{
									"ports": []map[string]interface{}{
										{"port": 8888},
									},
								},
								"proc2": map[string]interface{}{
									"ports": []map[string]interface{}{
										{"port": 8889},
										{"target_port": 5000},
									},
								},
								"proc.proc3": map[string]interface{}{
									"ports": []map[string]interface{}{
										{"port": 9000},
										{"target_port": 5001},
									},
								},
							},
						},
					},
				},
			},
			expectedProcesses: map[string][]string{},
			expectedPorts:     []string{},
			expectedYamlData: provTypes.TsuruYamlData{
				Hooks: &provTypes.TsuruYamlHooks{
					Build: []string{
						"script1",
						"script2",
					},
				},
				Healthcheck: &provTypes.TsuruYamlHealthcheck{
					Path: "/status",
				},
				Kubernetes: &provTypes.TsuruYamlKubernetesConfig{
					Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
						"pod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
							"proc1": {
								Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
									{Port: 8888},
								},
							},
							"proc2": {
								Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
									{Port: 8889},
									{TargetPort: 5000},
								},
							},
							"proc.proc3": {
								Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
									{Port: 9000},
									{TargetPort: 5001},
								},
							},
						},
					},
				},
			},
		},
	}
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)
	for _, tt := range tests {
		func() {
			c.Log("test", tt.name)
			version, err := svc.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
				App: &appTypes.MockApp{Name: "myapp"},
			})
			c.Assert(err, check.IsNil)
			err = version.AddData(tt.addData)
			c.Assert(err, check.IsNil)

			procs, err := version.Processes()
			c.Assert(err, check.IsNil)
			c.Check(procs, check.DeepEquals, tt.expectedProcesses)
			yamlData, err := version.TsuruYamlData()
			c.Assert(err, check.IsNil)
			c.Check(yamlData, check.DeepEquals, tt.expectedYamlData)
			c.Check(version.VersionInfo().ExposedPorts, check.DeepEquals, tt.expectedPorts)
			if tt.expectedRaw != nil {
				c.Check(version.VersionInfo().CustomData, check.DeepEquals, tt.expectedRaw)
			}
		}()
	}
}

func (s *S) TestAppVersionImpl_WebProcess(c *check.C) {
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)

	tests := []struct {
		procs   map[string][]string
		webProc string
	}{
		{
			webProc: "",
		},
		{
			procs: map[string][]string{
				"web":    {"python myapp.py"},
				"worker": {"someworker"},
			},
			webProc: "web",
		},
		{
			procs: map[string][]string{
				"worker1": {"python myapp.py"},
				"worker2": {"someworker"},
			},
			webProc: "worker1",
		},
		{
			procs: map[string][]string{
				"api": {"python myapi.py"},
			},
			webProc: "api",
		},
	}
	for i, tt := range tests {
		c.Logf("test %d", i)
		svc.DeleteVersions(context.TODO(), "myapp")
		version, err := svc.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
			App: &appTypes.MockApp{Name: "myapp"},
		})
		c.Assert(err, check.IsNil)
		err = version.AddData(appTypes.AddVersionDataArgs{
			Processes: tt.procs,
		})
		c.Assert(err, check.IsNil)
		webProc, err := version.WebProcess()
		c.Assert(err, check.IsNil)
		c.Assert(webProc, check.Equals, tt.webProc)
	}
}

func (s *S) TestAppVersionImpl_ToggleEnabled(c *check.C) {
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)
	version, err := svc.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: &appTypes.MockApp{Name: "myapp"},
	})
	c.Assert(err, check.IsNil)
	err = version.ToggleEnabled(false, "my reason")
	c.Assert(err, check.IsNil)
	c.Assert(version.VersionInfo().Disabled, check.Equals, true)
	c.Assert(version.VersionInfo().DisabledReason, check.Equals, "my reason")

	err = version.ToggleEnabled(true, "")
	c.Assert(err, check.IsNil)
	c.Assert(version.VersionInfo().Disabled, check.Equals, false)
	c.Assert(version.VersionInfo().DisabledReason, check.Equals, "")

	err = version.ToggleEnabled(false, "other reason")
	c.Assert(err, check.IsNil)
	c.Assert(version.VersionInfo().Disabled, check.Equals, true)
	c.Assert(version.VersionInfo().DisabledReason, check.Equals, "other reason")
}
