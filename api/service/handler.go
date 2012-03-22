package service

import (
	"fmt"
	"net/http"
	"strconv"
)

func CreateHandler(w http.ResponseWriter, r *http.Request) {
	appId, _ := strconv.Atoi(r.FormValue("ServiceTypeId"))
	service := Service{
		ServiceTypeId: int64(appId),
		Name:          r.FormValue("name"),
	}

	service.Create()
	fmt.Fprint(w, "success")
}

func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	s := Service{Name: r.URL.Query().Get(":name")}
	s.Get()
	s.Delete()
	fmt.Fprint(w, "success")
}
