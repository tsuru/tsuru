package api

import (
	"fmt"
	"net/http"
)

func CreateService(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "success")
}
