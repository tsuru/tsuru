package api

import (
	"fmt"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"net/http"
)

func platformAdd(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.FormValue("name")
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	w.Header().Set("Content-Type", "text")
	err := app.PlatformAdd(name, args, w)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\nOK!")
	return nil
}

func platformUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.URL.Query().Get(":name")
	err := r.ParseForm()
	if err != nil {
		return err
	}
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	w.Header().Set("Content-Type", "text")
	err = app.PlatformUpdate(name, args, w)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\nOK!")
	return nil
}

func platformRemove(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.URL.Query().Get(":name")
	return app.PlatformRemove(name)
}
