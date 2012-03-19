package app

import (
	"fmt"
	"net/http"
)

func CreateAppHandler(w http.ResponseWriter, r *http.Request) {
	//app = App{}
	//err := app.Create()
	//if err != nil {
	//	app.Destroy()
	//}
	fmt.Fprint(w, "success")
}
