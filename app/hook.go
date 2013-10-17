// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/repository"
	"io"
	"launchpad.net/goyaml"
	"path"
)

var errCannotLoadAppYAML = errors.New("Cannot load app.yaml/app.yml file.")

type hookRunner interface {
	Restart(app *App, w io.Writer, kind string) error
}

type yamlHookRunner struct {
	config *appConfig
}

type appConfig struct {
	Restart hook
	Build   []string
}

type hook struct {
	Before []string
	After  []string
}

func (r *yamlHookRunner) Build(app *App, w io.Writer) error {
	err := r.loadConfig(app)
	if err == errCannotLoadAppYAML {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (r *yamlHookRunner) Restart(app *App, w io.Writer, kind string) error {
	err := r.loadConfig(app)
	if err == errCannotLoadAppYAML {
		return nil
	} else if err != nil {
		return err
	}
	hooks := map[string][]string{
		"before": r.config.Restart.Before,
		"after":  r.config.Restart.After,
	}
	cmds := hooks[kind]
	if len(cmds) > 0 {
		fmt.Fprintf(w, " ---> Running restart:%s\n\n", kind)
		for _, cmd := range cmds {
			err := app.sourced(cmd, w, true)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *yamlHookRunner) loadConfig(app *App) error {
	if r.config != nil {
		return nil
	}
	repoPath, err := repository.GetPath()
	if err != nil {
		return err
	}
	err = r.loadConfigFromFile(app, path.Join(repoPath, "app.yaml"))
	if err != nil {
		err = r.loadConfigFromFile(app, path.Join(repoPath, "app.yml"))
	}
	return err
}

func (r *yamlHookRunner) loadConfigFromFile(app *App, filename string) error {
	var buf bytes.Buffer
	app.run("cat "+filename, &buf, true)
	var m map[string]appConfig
	goyaml.Unmarshal(buf.Bytes(), &m)
	if _, ok := m["hooks"]; !ok {
		r.config = &appConfig{}
		return errCannotLoadAppYAML
	}
	config := m["hooks"]
	r.config = &config
	return nil
}
