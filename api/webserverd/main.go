package main

import (
	"github.com/timeredbull/tsuru/api/app"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/services/create", app.CreateServiceHandler)
	http.HandleFunc("/apps", app.CreateAppHandler)
	log.Fatal(http.ListenAndServe(":4000", nil))
}
