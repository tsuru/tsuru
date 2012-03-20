package main

import (
	"github.com/timeredbull/tsuru/api/app"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/apps", app.CreateAppHandler)
	log.Fatal(http.ListenAndServe("localhost:4000", nil))
}
