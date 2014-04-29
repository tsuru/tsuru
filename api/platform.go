package api

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"net/http"
)

func platformAdd(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	name := r.FormValue("name")
	dockerfile := r.FormValue("dockerfile")
	err := app.PlatformAdd(name, dockerfile)
	if err != nil {
		return err
	}
	return nil
}
