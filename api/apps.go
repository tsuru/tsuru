package api

import (
	"fmt"
	"net/http"
)

type App struct {
	name string
	framework string
	runtime string
	state string
}

func updateAppFromParams() {

}

func (app *App) Create() (error) {
	return error
}

func CreateAppHandler(w http.ResponseWriter, r *http.Request) {
	//app = App{}
	//err := app.Create()
	//if err != nil {
	//	app.Destroy()
	//}
	fmt.Fprint(w, "success")
}

