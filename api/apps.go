package api

import (
	"fmt"
	"net/http"
)

func CreateAppHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "success")
}
