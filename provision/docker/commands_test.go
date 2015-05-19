// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
)

func (s *S) TestGitDeployCmds(c *check.C) {
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	hostEnv := bind.EnvVar{
		Name:   "TSURU_HOST",
		Value:  "tsuru_host",
		Public: true,
	}
	tokenEnv := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(hostEnv)
	app.SetEnv(tokenEnv)
	repository.Manager().CreateRepository("app-name", nil)
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, check.IsNil)
	expectedPart1 := fmt.Sprintf("%s git git://"+repositorytest.ServerHost+"/app-name.git version", deployCmd)
	expectedAgent := fmt.Sprintf(`tsuru_unit_agent tsuru_host app_token app-name "%s" deploy`, expectedPart1)
	cmds, err := gitDeployCmds(app, "version")
	c.Assert(err, check.IsNil)
	c.Assert(cmds, check.DeepEquals, []string{"/bin/bash", "-lc", expectedAgent})
}

func (s *S) TestArchiveDeployCmds(c *check.C) {
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	hostEnv := bind.EnvVar{
		Name:   "TSURU_HOST",
		Value:  "tsuru_host",
		Public: true,
	}
	tokenEnv := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(hostEnv)
	app.SetEnv(tokenEnv)
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, check.IsNil)
	archiveURL := "https://s3.amazonaws.com/wat/archive.tar.gz"
	expectedPart1 := fmt.Sprintf("%s archive %s", deployCmd, archiveURL)
	expectedAgent := fmt.Sprintf(`tsuru_unit_agent tsuru_host app_token app-name "%s" deploy`, expectedPart1)
	cmds, err := archiveDeployCmds(app, archiveURL)
	c.Assert(err, check.IsNil)
	c.Assert(cmds, check.DeepEquals, []string{"/bin/bash", "-lc", expectedAgent})
}

func (s *S) TestRunWithAgentCmds(c *check.C) {
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	hostEnv := bind.EnvVar{
		Name:   "TSURU_HOST",
		Value:  "tsuru_host",
		Public: true,
	}
	tokenEnv := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(hostEnv)
	app.SetEnv(tokenEnv)
	runCmd, err := config.GetString("docker:run-cmd:bin")
	c.Assert(err, check.IsNil)
	cmds, err := runWithAgentCmds(app)
	c.Assert(err, check.IsNil)
	c.Assert(cmds, check.DeepEquals, []string{"tsuru_unit_agent", "tsuru_host", "app_token", "app-name", runCmd})
}

func (s *S) TestRunLeanContainersCmd(c *check.C) {
	imageId := "tsuru/app-sample"
	customData := map[string]interface{}{
		"procfile": "web: python web.py",
	}
	err := saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := runLeanContainerCmds("web", imageId, nil)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "web")
	expected := []string{"/bin/bash", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec python web.py"}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRunLeanContainersCmdHooks(c *check.C) {
	imageId := "tsuru/app-sample"
	customData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"restart": map[string]interface{}{
				"before": []string{"cmd1", "cmd2"},
			},
		},
		"procfile": "web: python web.py",
	}
	err := saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := runLeanContainerCmds("web", imageId, nil)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "web")
	expected := []string{"/bin/bash", "-lc", "[ -d /home/application/current ] && cd /home/application/current; cmd1 && cmd2 && exec python web.py"}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRunLeanContainersCmdNoProcesses(c *check.C) {
	imageId := "tsuru/app-sample"
	customData := map[string]interface{}{}
	err := saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	hostEnv := bind.EnvVar{
		Name:   "TSURU_HOST",
		Value:  "tsuru_host",
		Public: true,
	}
	tokenEnv := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(hostEnv)
	app.SetEnv(tokenEnv)
	cmds, process, err := runLeanContainerCmds("web", imageId, app)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "")
	runCmd, err := config.GetString("docker:run-cmd:bin")
	c.Assert(err, check.IsNil)
	expected := []string{"tsuru_unit_agent", "tsuru_host", "app_token", "app-name", runCmd}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRunLeanContainersImplicitProcess(c *check.C) {
	imageId := "tsuru/app-sample"
	customData := map[string]interface{}{
		"procfile": "web: python web.py",
	}
	err := saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := runLeanContainerCmds("", imageId, nil)
	c.Assert(err, check.IsNil)
	c.Assert(process, check.Equals, "web")
	expected := []string{"/bin/bash", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec python web.py"}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRunLeanContainersCmdNoProcessSpecified(c *check.C) {
	imageId := "tsuru/app-sample"
	customData := map[string]interface{}{
		"procfile": "web: python web.py\nworker: python worker.py",
	}
	err := saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := runLeanContainerCmds("", imageId, nil)
	c.Assert(err, check.NotNil)
	e, ok := err.(provision.ErrInvalidProcess)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Msg, check.Equals, "no process name specified and more than one declared in Procfile")
	c.Assert(process, check.Equals, "")
	c.Assert(cmds, check.IsNil)
}

func (s *S) TestRunLeanContainersCmdInvalidProcess(c *check.C) {
	imageId := "tsuru/app-sample"
	customData := map[string]interface{}{
		"procfile": "web: python web.py",
	}
	err := saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	cmds, process, err := runLeanContainerCmds("worker", imageId, nil)
	c.Assert(err, check.NotNil)
	e, ok := err.(provision.ErrInvalidProcess)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Msg, check.Equals, `no command declared in Procfile for process "worker"`)
	c.Assert(process, check.Equals, "")
	c.Assert(cmds, check.IsNil)
}

func (s *S) TestRunLeanContainersCmdNoImageMetadata(c *check.C) {
	cmds, process, err := runLeanContainerCmds("web", "tsuru/app-myapp", nil)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
	c.Assert(process, check.Equals, "")
	c.Assert(cmds, check.IsNil)
}
