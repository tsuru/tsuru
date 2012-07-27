package provision

import (
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/errors"
    "net/http"
)

func GetServiceOrError(name string, u *auth.User) (service.Service, error) {
	s := service.Service{Name: name}
	err := s.Get()
	if err != nil {
		return s, &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !auth.CheckUserAccess(s.OwnerTeams, u) {
		msg := "This user does not have access to this service"
		return s, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	return s, err
}
