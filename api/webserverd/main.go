package main

import (
	"github.com/bmizerany/pat"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/service"
	"log"
	"net/http"
)

func main() {
	m := pat.New()

	m.Post("/services", http.HandlerFunc(service.CreateHandler))
	m.Get("/services/:name", http.HandlerFunc(service.DeleteHandler))
	m.Post("/services/bind", http.HandlerFunc(service.BindHandler))
	m.Post("/services/unbind", http.HandlerFunc(service.BindHandler))

	m.Get("/apps", http.HandlerFunc(app.AppList))
	m.Post("/apps", http.HandlerFunc(app.CreateAppHandler))
	m.Get("/apps/:name", http.HandlerFunc(app.AppInfo))
	m.Post("/apps/:name/application", http.HandlerFunc(app.Upload))

	log.Fatal(http.ListenAndServe(":4000", m))
}
