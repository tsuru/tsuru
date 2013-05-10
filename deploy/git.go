// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/repository"
	"io"
)

// Clone runs a git clone to clone the app repository in an ap.
func clone(p provision.Provisioner, app provision.App) ([]byte, error) {
	var buf bytes.Buffer
	path, err := repository.GetPath()
	if err != nil {
		return nil, fmt.Errorf("Tsuru is misconfigured: %s", err)
	}
	cmd := fmt.Sprintf("git clone %s %s --depth 1", repository.GetReadOnlyUrl(app.GetName()), path)
	err = p.ExecuteCommand(&buf, &buf, app, cmd)
	b := buf.Bytes()
	log.Printf(`"git clone" output: %s`, b)
	return b, err
}

// Pull runs a git pull to update the code in an app
//
// It works like Clone, pulling from the app bare repository.
func pull(p provision.Provisioner, app provision.App) ([]byte, error) {
	var buf bytes.Buffer
	path, err := repository.GetPath()
	if err != nil {
		return nil, fmt.Errorf("Tsuru is misconfigured: %s", err)
	}
	cmd := fmt.Sprintf("cd %s && git pull origin master", path)
	err = p.ExecuteCommand(&buf, &buf, app, cmd)
	b := buf.Bytes()
	log.Printf(`"git pull" output: %s`, b)
	return b, err
}

func Git(provisioner provision.Provisioner, app provision.App, w io.Writer) error {
	log.Write(w, []byte("\n ---> Tsuru receiving push\n"))
	log.Write(w, []byte("\n ---> Replicating the application repository across units\n"))
	out, err := clone(provisioner, app)
	if err != nil {
		out, err = pull(provisioner, app)
	}
	if err != nil {
		msg := fmt.Sprintf("Got error while clonning/pulling repository: %s -- \n%s", err.Error(), string(out))
		log.Write(w, []byte(msg))
		return errors.New(msg)
	}
	log.Write(w, out)
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
