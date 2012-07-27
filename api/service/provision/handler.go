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

func DeleteHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	s, err := service.GetServiceOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	n, err := db.Session.ServiceInstances().Find(bson.M{"service_name": s.Name}).Count()
	if err != nil {
		return err
	}
	if n > 0 {
		msg := "This service cannot be removed because it has instances.\nPlease remove these instances before removing the service."
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	err = s.Delete()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func getServiceAndTeamOrError(serviceName string, teamName string, u *auth.User) (*service.Service, *auth.Team, error) {
	service := &service.Service{Name: serviceName}
	err := service.Get()
	if err != nil {
		return nil, nil, &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !auth.CheckUserAccess(service.Teams, u) {
		msg := "This user does not have access to this service"
		return nil, nil, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	t := new(auth.Team)
	err = db.Session.Teams().Find(bson.M{"name": teamName}).One(t)
	if err != nil {
		return nil, nil, &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	return service, t, nil
}

func GrantAccessToTeamHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	service, t, err := getServiceAndTeamOrError(r.URL.Query().Get(":service"), r.URL.Query().Get(":team"), u)
	if err != nil {
		return err
	}
	err = service.GrantAccess(t)
	if err != nil {
		return &errors.Http{Code: http.StatusConflict, Message: err.Error()}
	}
	return db.Session.Services().Update(bson.M{"_id": service.Name}, service)
}

func RevokeAccessFromTeamHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	service, t, err := getServiceAndTeamOrError(r.URL.Query().Get(":service"), r.URL.Query().Get(":team"), u)
	if err != nil {
		return err
	}
	if len(service.Teams) < 2 {
		msg := "You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	err = service.RevokeAccess(t)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: err.Error()}
	}
	return db.Session.Services().Update(bson.M{"_id": service.Name}, service)
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
