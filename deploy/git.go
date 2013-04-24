package deploy

import (
	"errors"
	"fmt"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/repository"
	"io"
)

type GitDeployer struct{}

func (d *GitDeployer) Deploy(app App, w io.Writer) error {
	if err := log.Write(w, []byte("\n ---> Tsuru receiving push\n")); err != nil {
		return err
	}
	if err := log.Write(w, []byte("\n ---> Replicating the application repository across units\n")); err != nil {
		return err
	}
	out, err := repository.CloneOrPull(app)
	if err != nil {
		msg := fmt.Sprintf("Got error while clonning/pulling repository: %s -- \n%s", err.Error(), string(out))
		return errors.New(msg)
	}
	if err := log.Write(w, out); err != nil {
		return err
	}
	if err := log.Write(w, []byte("\n ---> Installing dependencies\n")); err != nil {
		return err
	}
	if err := app.InstallDeps(w); err != nil {
		return err
	}
	if err := app.Restart(w); err != nil {
		return err
	}
	return log.Write(w, []byte("\n ---> Deploy done!\n\n"))
}
