package api

import (
	"fmt"
	"net/http"
)

type ServiceBindings struct {
	ServiceConfigId int
	AppId int
	UserId int
	BindingToken int
	Name string
	Configuration string
	Credentials string
	BindingOptions string
	CreatedAt string //fix
	UpdatedAt string //fix
}

func CreateService(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "success")
}
