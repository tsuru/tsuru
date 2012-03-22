package main

import (
	"code.google.com/p/gorilla/mux"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/service"
	"log"
	"net/http"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/services/create", service.CreateServiceHandler)
	r.HandleFunc("/apps", app.CreateAppHandler)
	http.Handle("/", r)
	log.Fatal(http.ListenAndServe(":4000", nil))
}
