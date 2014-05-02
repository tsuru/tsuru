package api

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"net/http"
)

func platformAdd(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	name := r.FormValue("name")
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	w.Header().Set("Content-Type", "text")
	return app.PlatformAdd(name, args, w)
}
