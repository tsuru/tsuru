package service

import (
	"fmt"
	"net/http"
	"strconv"
)

func CreateService(w http.ResponseWriter, r *http.Request) {
	serviceConfigId, _ := strconv.Atoi(r.FormValue("ServiceConfigId"))
	appId, _ := strconv.Atoi(r.FormValue("AppId"))
	userId, _ := strconv.Atoi(r.FormValue("UserId"))
	bindingTokenId, _ := strconv.Atoi(r.FormValue("BindingTokenId"))

	service := ServiceBinding{
		ServiceConfigId: serviceConfigId,
		AppId:			 appId,
		UserId:			 userId,
		BindingTokenId:	 bindingTokenId,
		Name:			 r.FormValue("name"),
	}
	service.Create()

	fmt.Fprint(w, "success")
}
