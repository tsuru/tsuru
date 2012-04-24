// +build ignore

package main

import (
	"."
	"github.com/bmizerany/pat"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/api/user"
	. "github.com/timeredbull/tsuru/database"
	"launchpad.net/mgo"
	"log"
	"net/http"
)

func main() {
	session, err := mgo.Dial("localhost:27017")
	if err != nil {
		panic(err)
	}
	Db = session.DB("tsuru")
	defer session.Close()
	m := pat.New()

	m.Post("/services", webserver.Handler(service.CreateHandler))
	m.Get("/services", webserver.Handler(service.ServicesHandler))
	m.Get("/services/types", webserver.Handler(service.ServiceTypesHandler))
	m.Get("/services/:name", webserver.Handler(service.DeleteHandler))
	m.Post("/services/bind", webserver.Handler(service.BindHandler))
	m.Post("/services/unbind", webserver.Handler(service.UnbindHandler))

	m.Get("/apps/:name/delete", webserver.Handler(app.AppDelete))
	m.Get("/apps/:name", webserver.Handler(app.AppInfo))
	m.Post("/apps/:name/application", webserver.Handler(app.Upload))
	m.Get("/apps", webserver.Handler(app.AppList))
	m.Post("/apps", webserver.Handler(app.CreateAppHandler))

	m.Post("/users", webserver.Handler(user.CreateUser))
	m.Post("/users/:email/tokens", webserver.Handler(user.Login))
	m.Get("/users/check-authorization", webserver.Handler(user.CheckAuthorization))

	log.Fatal(http.ListenAndServe(":4000", m))
}
