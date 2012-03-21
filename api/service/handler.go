package service

import (
	"fmt"
	"net/http"
	"strconv"
)

func CreateServiceHandler(w http.ResponseWriter, r *http.Request) {
	appId, _ := strconv.Atoi(r.FormValue("AppId"))
	service := Service{
		AppId:           appId,
		Name:            r.FormValue("name"),
	}

	service.Create()
	fmt.Fprint(w, "success")
}
