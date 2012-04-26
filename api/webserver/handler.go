package webserver

import (
	"github.com/timeredbull/tsuru/log"
	"net/http"
)

type Handler func(http.ResponseWriter, *http.Request) error

func (fn Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), 500)
		log.Print(err.Error())
	}
}
