package service

import (
	"fmt"
	"net/http"
	"strconv"
)

func CreateServiceHandler(w http.ResponseWriter, r *http.Request) {
	appId, _ := strconv.Atoi(r.FormValue("ServiceTypeId"))
	service := Service{
		ServiceTypeId:   int64(appId),
		Name:            r.FormValue("name"),
	}

	service.Create()
	fmt.Fprint(w, "success")
}
