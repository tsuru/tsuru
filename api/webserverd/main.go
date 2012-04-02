// +build ignore

package main

import (
	"."
	"database/sql"
	"github.com/bmizerany/pat"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/api/user"
	. "github.com/timeredbull/tsuru/database"
	"log"
	"net/http"
)

func main() {
	Db, _ = sql.Open("sqlite3", "./tsuru.db")
	defer Db.Close()
	m := pat.New()

	m.Post("/services", webserverd.Handler(service.CreateHandler))
	m.Get("/services", webserverd.Handler(service.ServicesHandler))
	m.Get("/services/types", webserverd.Handler(service.ServiceTypesHandler))
	m.Get("/services/:name", webserverd.Handler(service.DeleteHandler))
	m.Post("/services/bind", webserverd.Handler(service.BindHandler))
	m.Post("/services/unbind", webserverd.Handler(service.UnbindHandler))

	m.Get("/apps/:name/delete", webserverd.Handler(app.AppDelete))
	m.Get("/apps/:name", webserverd.Handler(app.AppInfo))
	m.Post("/apps/:name/application", webserverd.Handler(app.Upload))
	m.Get("/apps", webserverd.Handler(app.AppList))
	m.Post("/apps", webserverd.Handler(app.CreateAppHandler))

	m.Post("/users", webserverd.Handler(user.CreateUser))

	log.Fatal(http.ListenAndServe(":4000", m))
}
