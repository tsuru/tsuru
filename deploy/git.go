// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package deploy

import (
	"errors"
	"fmt"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/repository"
	"io"
)

func Git(provisioner provision.Provisioner, app provision.App, w io.Writer) error {
	log.Write(w, []byte("\n ---> Tsuru receiving push\n"))
	log.Write(w, []byte("\n ---> Replicating the application repository across units\n"))
	out, err := repository.CloneOrPull(app)
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
