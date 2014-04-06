// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/repository"
	"io"
)

// Clone runs a git clone to clone the app repository in an app.
func clone(p provision.Provisioner, app provision.App) ([]byte, error) {
	var buf bytes.Buffer
	path, err := repository.GetPath()
	if err != nil {
		return nil, fmt.Errorf("Tsuru is misconfigured: %s", err)
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
		return nil, fmt.Errorf("Tsuru is misconfigured: %s", err)
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
		return nil, fmt.Errorf("Tsuru is misconfigured: %s", err)
	}
	cmd := fmt.Sprintf("cd %s && git checkout %s", path, version)
	if err := p.ExecuteCommand(&buf, &buf, app, cmd); err != nil {
		return buf.Bytes(), err
	}
	return nil, nil
}

func Git(provisioner provision.Provisioner, app provision.App, objID string, w io.Writer) error {
	log.Write(w, []byte("\n ---> Tsuru receiving push\n"))
	log.Write(w, []byte("\n ---> Replicating the application repository across units\n"))
	out, err := clone(provisioner, app)
	if err != nil {
		out, err = fetch(provisioner, app)
	}
	if err != nil {
		msg := fmt.Sprintf("Got error while cloning/fetching repository: %s -- \n%s", err, string(out))
		log.Write(w, []byte(msg))
		return errors.New(msg)
	}
	out, err = checkout(provisioner, app, objID)
	if err != nil {
		msg := fmt.Sprintf("Failed to checkout Git repository: %s -- \n%s", err, string(out))
		log.Write(w, []byte(msg))
		return errors.New(msg)
	}
	log.Write(w, []byte("\n ---> Installing dependencies\n"))
	if err := provisioner.InstallDeps(app, w); err != nil {
		log.Write(w, []byte(err.Error()))
		return err
	}
	log.Write(w, []byte("\n ---> Restarting application\n"))
	if err := app.Restart(w); err != nil {
		log.Write(w, []byte(err.Error()))
		return err
	}
	return log.Write(w, []byte("\n ---> Deploy done!\n\n"))
}
