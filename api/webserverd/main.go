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
	m.Post("/services/create", http.HandlerFunc(service.CreateServiceHandler))
	m.Post("/apps", http.HandlerFunc(app.CreateAppHandler))
	m.Get("/apps/:name", http.HandlerFunc(app.AppInfo))
	log.Fatal(http.ListenAndServe(":4000", m))
}
