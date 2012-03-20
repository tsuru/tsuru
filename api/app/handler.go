package app

import (
	"fmt"
	"net/http"
)

func CreateAppHandler(w http.ResponseWriter, r *http.Request) {
	app := App{Name: r.FormValue("name"), Framework: r.FormValue("framework")}
	err := app.Create()
	if err != nil {
		panic(err)
	}
	fmt.Fprint(w, "success")
}
