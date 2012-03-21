package main

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/service"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/services/create", service.CreateServiceHandler)
	http.HandleFunc("/apps", app.CreateAppHandler)
	log.Fatal(http.ListenAndServe(":4000", nil))
}
