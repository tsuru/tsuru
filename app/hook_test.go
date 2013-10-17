// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"github.com/globocom/config"
	"io"
	"launchpad.net/gocheck"
)

func (s *S) TestYAMLHookLoadConfig(c *gocheck.C) {
	output := `hooks:
  restart:
    before:
      - python manage.py collectstatic
      - python manage.py migrate
    before-each:
      - python manage.py download-manifest
    after:
      - python manage.py clear-cache
  build:
    - python manage.py validate
`
	s.provisioner.PrepareOutput([]byte(output))
	var runner yamlHookRunner
	app := App{Name: "beside"}
	err := runner.loadConfig(&app)
	c.Assert(err, gocheck.IsNil)
	expected := appConfig{
		Restart: hook{
			Before: []string{
				"python manage.py collectstatic",
				"python manage.py migrate",
			},
			After: []string{"python manage.py clear-cache"},
		},
		Build: []string{"python manage.py validate"},
	}
	c.Assert(*runner.config, gocheck.DeepEquals, expected)
	cmds := s.provisioner.GetCmds("cat /home/application/current/app.yaml", &app)
	c.Assert(cmds, gocheck.HasLen, 1)
}

func (s *S) TestYAMLLoadConfigCaches(c *gocheck.C) {
	config := appConfig{
		Restart: hook{
			Before: []string{
				"python manage.py collectstatic",
				"python manage.py migrate",
			},
			After: []string{"python manage.py clear-cache"},
		},
	}
	var runner yamlHookRunner
	runner.config = &config
	app := App{Name: "beside"}
	err := runner.loadConfig(&app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(runner.config, gocheck.Equals, &config)
	c.Assert(s.provisioner.GetCmds("", &app), gocheck.HasLen, 0)
}

func (s *S) TestYAMLLoadConfigAppYML(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("Not yam ::: l!"))
	output := `hooks:
  restart:
    before:
      - python manage.py collectstatic
      - python manage.py migrate
    before-each:
      - python manage.py download-manifest
    after:
      - python manage.py clear-cache
`
	s.provisioner.PrepareOutput([]byte(output))
	var runner yamlHookRunner
	app := App{Name: "beside"}
	err := runner.loadConfig(&app)
	c.Assert(err, gocheck.IsNil)
	expected := appConfig{
		Restart: hook{
			Before: []string{
				"python manage.py collectstatic",
				"python manage.py migrate",
			},
			After: []string{"python manage.py clear-cache"},
		},
	}
	c.Assert(*runner.config, gocheck.DeepEquals, expected)
	cmds := s.provisioner.GetCmds("cat /home/application/current/app.yaml", &app)
	c.Assert(cmds, gocheck.HasLen, 1)
	cmds = s.provisioner.GetCmds("cat /home/application/current/app.yml", &app)
	c.Assert(cmds, gocheck.HasLen, 1)
}

func (s *S) TestYAMLLoadConfigInvalid(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("Not yam ::: l!"))
	var runner yamlHookRunner
	app := App{Name: "beside"}
	err := runner.loadConfig(&app)
	c.Assert(err, gocheck.Equals, errCannotLoadAppYAML)
	c.Assert(runner.config, gocheck.NotNil)
}

func (s *S) TestYAMLRunnerRestartBefore(c *gocheck.C) {
	app := App{
		Name: "kn",
		Units: []Unit{
			{Name: "kn/0"}, {Name: "kn/1"}, {Name: "kn/2"},
		},
	}
	s.provisioner.PrepareOutput([]byte(". .."))
	s.provisioner.PrepareOutput([]byte("nothing"))
	runner := yamlHookRunner{
		config: &appConfig{
			Restart: hook{Before: []string{"ls -a", "cat /home/application/apprc"}},
		},
	}
	var buf bytes.Buffer
	err := runner.Restart(&app, &buf, "before")
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, " ---> Running restart:before\n\n. ..nothing")
	cmds := s.provisioner.GetCmds("", &app)
	c.Assert(cmds, gocheck.HasLen, 2)
	c.Check(cmds[0].Cmd, gocheck.Matches, `.*source /home/application/apprc.*ls -a$`)
	c.Check(cmds[1].Cmd, gocheck.Matches, `.*source /home/application/apprc.*cat /home/application/apprc$`)
}

func (s *S) TestYAMLRunnerRestartAfter(c *gocheck.C) {
	app := App{
		Name: "kn",
		Units: []Unit{
			{Name: "kn/0"}, {Name: "kn/1"}, {Name: "kn/2"},
		},
	}
	s.provisioner.PrepareOutput([]byte(". .."))
	runner := yamlHookRunner{
		config: &appConfig{
			Restart: hook{After: []string{"ls -la"}},
		},
	}
	var buf bytes.Buffer
	err := runner.Restart(&app, &buf, "after")
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, " ---> Running restart:after\n\n. ..")
	cmds := s.provisioner.GetCmds("", &app)
	c.Assert(cmds, gocheck.HasLen, 1)
	c.Check(cmds[0].Cmd, gocheck.Matches, `.*source /home/application/apprc.*ls -la$`)
}

func (s *S) TestYAMLRunnerLoadsConfig(c *gocheck.C) {
	app := App{Name: "kn"}
	yaml := `hooks:
  restart:
    before:
      - ls -a
      - cat /home/application/apprc
`
	s.provisioner.PrepareOutput([]byte(yaml))
	s.provisioner.PrepareOutput([]byte(". .."))
	s.provisioner.PrepareOutput([]byte("nothing"))
	var runner yamlHookRunner
	var buf bytes.Buffer
	err := runner.Restart(&app, &buf, "before")
	c.Assert(err, gocheck.IsNil)
	c.Check(buf.String(), gocheck.Equals, " ---> Running restart:before\n\n. ..nothing")
	cmds := s.provisioner.GetCmds("", &app)
	c.Assert(cmds, gocheck.HasLen, 3)
	c.Check(cmds[1].Cmd, gocheck.Matches, `.*source /home/application/apprc.*ls -a$`)
	c.Check(cmds[2].Cmd, gocheck.Matches, `.*source /home/application/apprc.*cat /home/application/apprc$`)
}

func (s *S) TestYAMLRunnerAbortsWhenCantLoadAppConfig(c *gocheck.C) {
	app := App{Name: "kn"}
	output := "not really an yaml"
	s.provisioner.PrepareOutput([]byte(output))
	var runner yamlHookRunner
	var buf bytes.Buffer
	err := runner.Restart(&app, &buf, "before")
	c.Assert(err, gocheck.IsNil)
	cmds := s.provisioner.GetCmds("", &app)
	c.Assert(cmds, gocheck.HasLen, 2)
	c.Assert(buf.String(), gocheck.Equals, "")
}

func (s *S) TestYAMLRunnerFailureInLoadConfig(c *gocheck.C) {
	app := App{Name: "shot"}
	old, _ := config.Get("git:unit-repo")
	config.Unset("git:unit-repo")
	defer config.Set("git:unit-repo", old)
	var runner yamlHookRunner
	var buf bytes.Buffer
	err := runner.Restart(&app, &buf, "before")
	c.Assert(err, gocheck.NotNil)
	c.Assert(buf.String(), gocheck.Equals, "")
}

func (s *S) TestBuildRunnerAbortsWhenCantLoadAppConfig(c *gocheck.C) {
	app := App{Name: "kn"}
	output := "not really an yaml"
	s.provisioner.PrepareOutput([]byte(output))
	var runner yamlHookRunner
	var buf bytes.Buffer
	err := runner.Build(&app, &buf)
	c.Assert(err, gocheck.IsNil)
	cmds := s.provisioner.GetCmds("", &app)
	c.Assert(cmds, gocheck.HasLen, 2)
	c.Assert(buf.String(), gocheck.Equals, "")
}

func (s *S) TestBuildRunnerFailureInLoadConfig(c *gocheck.C) {
	app := App{Name: "shot"}
	old, _ := config.Get("git:unit-repo")
	config.Unset("git:unit-repo")
	defer config.Set("git:unit-repo", old)
	var runner yamlHookRunner
	var buf bytes.Buffer
	err := runner.Build(&app, &buf)
	c.Assert(err, gocheck.NotNil)
	c.Assert(buf.String(), gocheck.Equals, "")
}

type fakeHookRunner struct {
	calls  map[string]int
	result func(string) error
}

func (r *fakeHookRunner) call(kind string) {
	if r.calls == nil {
		r.calls = make(map[string]int)
	}
	r.calls[kind] = r.calls[kind] + 1
}

func (r *fakeHookRunner) Restart(app *App, w io.Writer, kind string) error {
	r.call(kind)
	if r.result != nil {
		return r.result(kind)
	}
	return nil
}
