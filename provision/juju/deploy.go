// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bytes"
	"fmt"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/repository"
)

// Clone runs a git clone to clone the app repository in an app.
func clone(p provision.Provisioner, app provision.App) ([]byte, error) {
	var buf bytes.Buffer
	path, err := repository.GetPath()
	if err != nil {
		return nil, fmt.Errorf("tsuru is misconfigured: %s", err)
	}
	cmd := fmt.Sprintf("git clone %s %s --depth 1", repository.ReadOnlyURL(app.GetName()), path)
	err = p.ExecuteCommand(&buf, &buf, app, cmd)
	b := buf.Bytes()
	log.Debugf(`"git clone" output: %s`, b)
	return b, err
}

// fetch runs a git fetch to update the code in the app.
//
// It works like Clone, fetching from the app remote repository.
func fetch(p provision.Provisioner, app provision.App) ([]byte, error) {
	var buf bytes.Buffer
	path, err := repository.GetPath()
	if err != nil {
		return nil, fmt.Errorf("tsuru is misconfigured: %s", err)
	}
	cmd := fmt.Sprintf("cd %s && git fetch origin", path)
	err = p.ExecuteCommand(&buf, &buf, app, cmd)
	b := buf.Bytes()
	log.Debugf(`"git fetch" output: %s`, b)
	return b, err
}

// checkout updates the Git repository of the app to the given version.
func checkout(p provision.Provisioner, app provision.App, version string) ([]byte, error) {
	var buf bytes.Buffer
	path, err := repository.GetPath()
	if err != nil {
		return nil, fmt.Errorf("tsuru is misconfigured: %s", err)
	}
	cmd := fmt.Sprintf("cd %s && git checkout %s", path, version)
	if err := p.ExecuteCommand(&buf, &buf, app, cmd); err != nil {
		return buf.Bytes(), err
	}
	return nil, nil
}
