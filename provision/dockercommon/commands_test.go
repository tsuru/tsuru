// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon_test

import (
	"fmt"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

type S struct {
	conn *db.Storage
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "provision_dockercommon_tests_s")
	config.Set("docker:registry", "my.registry")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
}

func (s *S) TestArchiveBuildCmds(c *check.C) {
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	config.Set("host", "tsuru_host")
	defer config.Unset("host")
	tokenEnv := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(tokenEnv)
	archiveURL := "https://s3.amazonaws.com/wat/archive.tar.gz"
	expectedPart1 := fmt.Sprintf("/var/lib/tsuru/deploy archive %s", archiveURL)
	expectedAgent := fmt.Sprintf(`tsuru_unit_agent tsuru_host app_token app-name "%s" build`, expectedPart1)
	cmds := dockercommon.ArchiveBuildCmds(app, archiveURL)
	c.Assert(cmds, check.DeepEquals, []string{"/bin/sh", "-lc", expectedAgent})
}

func (s *S) TestArchiveDeployCmds(c *check.C) {
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	config.Set("host", "tsuru_host")
	defer config.Unset("host")
	tokenEnv := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(tokenEnv)
	archiveURL := "https://s3.amazonaws.com/wat/archive.tar.gz"
	expectedPart1 := fmt.Sprintf("/var/lib/tsuru/deploy archive %s", archiveURL)
	expectedAgent := fmt.Sprintf(`tsuru_unit_agent tsuru_host app_token app-name "%s" deploy`, expectedPart1)
	cmds := dockercommon.ArchiveDeployCmds(app, archiveURL)
	c.Assert(cmds, check.DeepEquals, []string{"/bin/sh", "-lc", expectedAgent})
}

func (s *S) TestDeployCmds(c *check.C) {
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	config.Set("host", "tsuru_host")
	defer config.Unset("host")
	tokenEnv := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(tokenEnv)
	expectedAgent := "tsuru_unit_agent tsuru_host app_token app-name deploy-only"
	cmds := dockercommon.DeployCmds(app)
	c.Assert(cmds, check.DeepEquals, []string{"/bin/sh", "-lc", expectedAgent})
}

func (s *S) TestRunLeanContainersCmd(c *check.C) {
	imageID := "tsuru/app-sample"
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python web.py",
		},
	}
	err := image.SaveImageCustomData(imageID, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := dockercommon.LeanContainerCmds("web", imageID, nil)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "web")
	expected := []string{"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec python web.py"}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRunLeanContainersCmdHooks(c *check.C) {
	imageID := "tsuru/app-sample"
	customData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"restart": map[string]interface{}{
				"before": []string{"cmd1", "cmd2"},
			},
		},
		"processes": map[string]interface{}{
			"web": "python web.py",
		},
	}
	err := image.SaveImageCustomData(imageID, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := dockercommon.LeanContainerCmds("web", imageID, nil)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "web")
	expected := []string{"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; cmd1 && cmd2 && exec python web.py"}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRunLeanContainersCmdNoProcesses(c *check.C) {
	imageID := "tsuru/app-sample"
	customData := map[string]interface{}{}
	err := image.SaveImageCustomData(imageID, customData)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	config.Set("host", "tsuru_host")
	defer config.Unset("host")
	tokenEnv := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(tokenEnv)
	cmds, process, err := dockercommon.LeanContainerCmds("", imageID, app)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "")
	expected := []string{"tsuru_unit_agent", "tsuru_host", "app_token", "app-name", "/var/lib/tsuru/start"}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRunLeanContainersImplicitProcess(c *check.C) {
	imageID := "tsuru/app-sample"
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python web.py",
		},
	}
	err := image.SaveImageCustomData(imageID, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := dockercommon.LeanContainerCmds("", imageID, nil)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "web")
	expected := []string{"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec python web.py"}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRunLeanContainersCmdNoProcessSpecified(c *check.C) {
	imageID := "tsuru/app-sample"
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	err := image.SaveImageCustomData(imageID, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := dockercommon.LeanContainerCmds("", imageID, nil)
	c.Assert(err, check.NotNil)
	e, ok := err.(provision.InvalidProcessError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Msg, check.Equals, "no process name specified and more than one declared in Procfile")
	c.Assert(process, check.Equals, "")
	c.Assert(cmds, check.IsNil)
}

func (s *S) TestRunLeanContainersCmdInvalidProcess(c *check.C) {
	imageID := "tsuru/app-sample"
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python web.py",
		},
	}
	err := image.SaveImageCustomData(imageID, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := dockercommon.LeanContainerCmds("worker", imageID, nil)
	c.Assert(err, check.NotNil)
	e, ok := err.(provision.InvalidProcessError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Msg, check.Equals, `no command declared in Procfile for process "worker"`)
	c.Assert(process, check.Equals, "")
	c.Assert(cmds, check.IsNil)
}

func (s *S) TestRunLeanContainersCmdNoImageMetadata(c *check.C) {
	cmds, process, err := dockercommon.LeanContainerCmds("web", "tsuru/app-myapp", nil)
	c.Assert(err, check.FitsTypeOf, provision.InvalidProcessError{})
	c.Assert(err, check.ErrorMatches, `.*no command declared in Procfile for process "web"`)
	c.Assert(process, check.Equals, "")
	c.Assert(cmds, check.IsNil)
}

func (s *S) TestLeanContainerCmdsManyCmds(c *check.C) {
	imageID := "tsuru/app-sample"
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": []string{"python", "web.py"},
		},
	}
	err := image.SaveImageCustomData(imageID, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := dockercommon.LeanContainerCmds("", imageID, nil)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "web")
	expected := []string{"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec $0 \"$@\"", "python", "web.py"}
	c.Assert(cmds, check.DeepEquals, expected)
}
