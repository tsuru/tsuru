package provision

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	"net/http"
)

type serviceYaml struct {
	Id        string
	Endpoint  map[string]string
	Bootstrap map[string]string
}

func AddDocHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	s, err := service.GetServiceOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	s.Doc = string(body)
	if err = s.Update(); err != nil {
		return err
	}
	return nil
}

func GetDocHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	s, err := service.GetServiceOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	w.Write([]byte(s.Doc))
	return nil
}

func CreateHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var sy serviceYaml
	err = goyaml.Unmarshal(body, &sy)
	if err != nil {
		return err
	}
	var teams []auth.Team
	db.Session.Teams().Find(bson.M{"users.email": u.Email}).All(&teams)
	if len(teams) == 0 {
		msg := "In order to create a service, you should be member of at least one team"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	n, err := db.Session.Services().Find(bson.M{"_id": sy.Id}).Count()
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	if n != 0 {
		msg := fmt.Sprintf("Service with name %s already exists.", sy.Id)
		return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	s := service.Service{
		Name:       sy.Id,
		Endpoint:   sy.Endpoint,
		Bootstrap:  sy.Bootstrap,
		OwnerTeams: auth.GetTeamsNames(teams),
	}
	err = s.Create()
	if err != nil {
		return err
	}
	fmt.Fprint(w, "success")
	return nil
}

func UpdateHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var yaml serviceYaml
	goyaml.Unmarshal(body, &yaml)
	s, err := service.GetServiceOrError(yaml.Id, u)
	if err != nil {
		return err
	}
	s.Endpoint = yaml.Endpoint
	s.Bootstrap = yaml.Bootstrap
	if err = s.Update(); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
