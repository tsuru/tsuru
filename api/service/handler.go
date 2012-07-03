package service

import (
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/ec2"
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

type bindJson struct {
	App     string
	Service string
}

// a service with a pointer to it's type
type serviceT struct {
	Name string
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
	s := Service{
		Name:      sy.Id,
		Endpoint:  sy.Endpoint,
		Bootstrap: sy.Bootstrap,
		Teams:     auth.GetTeamsNames(teams),
	}
	s.Create()
	fmt.Fprint(w, "success")
	return nil
}

func CreateInstanceHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	var jService map[string]string
	err = json.Unmarshal(b, &jService)
	if err != nil {
		panic(err)
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	var s Service
	err = db.Session.Services().Find(bson.M{"_id": jService["service_name"]}).One(&s)
	if err != nil {
		msg := err.Error()
		if msg == "not found" {
			msg = fmt.Sprintf("Service %s does not exists.", jService["service_name"])
		}
		return &errors.Http{Code: http.StatusNotFound, Message: msg}
	}
	var teams []auth.Team
	err = db.Session.Teams().Find(bson.M{"users.email": u.Email}).All(&teams)
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	q := bson.M{"_id": jService["service_name"], "teams": bson.M{"$in": auth.GetTeamsNames(teams)}}
	n, err := db.Session.Services().Find(q).Count()
	if n == 0 {
		msg := fmt.Sprintf("You don't have access to service %s", jService["service_name"])
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	instance := ""
	if s.Bootstrap["when"] == OnNewInstance {
		instance, err = ec2.RunInstance(s.Bootstrap["ami"], "") //missing user data
		if err != nil {
			msg := fmt.Sprintf("Instance for service could not be created. \nError: %s", err.Error())
			return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
		}
	}
	si := ServiceInstance{
		Name:        jService["name"],
		ServiceName: jService["service_name"],
		Instance:    instance,
	}
	var cli *Client
	if cli, err = s.GetClient("production"); err == nil {
		si.Env, err = cli.Create(&si)
		if err != nil {
			return err
		}
	}
	return si.Create()
}

func DeleteHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	s := Service{Name: r.URL.Query().Get(":name")}
	err := s.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !s.CheckUserAccess(u) {
		msg := "This user does not have access to this service"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	s.Delete()
	fmt.Fprint(w, "success")
	return nil
}

// func BindHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
// 	var b bindJson
// 	defer r.Body.Close()
// 	body, err := ioutil.ReadAll(r.Body)
// 	if err != nil {
// 		return err
// 	}
// 	err = json.Unmarshal(body, &b)
// 	if err != nil {
// 		return err
// 	}
// 	s := Service{Name: b.Service}
// 	err = s.Get()
// 	if err != nil {
// 		return &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
// 	}
// 	if !s.CheckUserAccess(u) {
// 		return &errors.Http{Code: http.StatusForbidden, Message: "This user does not have access to this service"}
// 	}
// 	a := app.App{Name: b.App}
// 	err = a.Get()
// 	if err != nil {
// 		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
// 	}
// 	if !a.CheckUserAccess(u) {
// 		return &errors.Http{Code: http.StatusForbidden, Message: "This user does not have access to this app"}
// 	}
// 	err = s.Bind(&a)
// 	if err != nil {
// 		return err
// 	}
// 	fmt.Fprint(w, "success")
// 	return nil
// }
// 
// func UnbindHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
// 	var b bindJson
// 	defer r.Body.Close()
// 	body, err := ioutil.ReadAll(r.Body)
// 	if err != nil {
// 		return err
// 	}
// 	err = json.Unmarshal(body, &b)
// 	if err != nil {
// 		return err
// 	}
// 	s := Service{Name: b.Service}
// 	err = s.Get()
// 	if err != nil {
// 		return &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
// 	}
// 	if !s.CheckUserAccess(u) {
// 		return &errors.Http{Code: http.StatusForbidden, Message: "This user does not have access to this service"}
// 	}
// 	a := app.App{Name: b.App}
// 	err = a.Get()
// 	if err != nil {
// 		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
// 	}
// 	if !a.CheckUserAccess(u) {
// 		return &errors.Http{Code: http.StatusForbidden, Message: "This user does not have access to this app"}
// 	}
// 	err = s.Unbind(&a)
// 	if err != nil {
// 		return err
// 	}
// 	fmt.Fprint(w, "success")
// 	return nil
// }

func getServiceAndTeamOrError(serviceName string, teamName string, u *auth.User) (*Service, *auth.Team, error) {
	service := &Service{Name: serviceName}
	err := service.Get()
	if err != nil {
		return nil, nil, &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !service.CheckUserAccess(u) {
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

func ServicesHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	set := NewSet()
	for _, team := range teams {
		teamApps, err := app.GetApps(&team)
		if err != nil {
			continue
		}
		for _, a := range teamApps {
			set.Add(a.Name)
		}
	}
	var teamNames []string
	for _, team := range teams {
		teamNames = append(teamNames, team.Name)
	}
	response := make(map[string][]string)
	var services []Service
	err = db.Session.Services().Find(bson.M{"teams": bson.M{"$in": teamNames}}).All(&services)
	if err != nil {
		return err
	}
	if len(services) == 0 {
		w.Write([]byte("null"))
		return nil
	}
	for _, service := range services {
		response[service.Name] = []string{}
	}
	iter := db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": set.Items()}}).Iter()
	var instance ServiceInstance
	for iter.Next(&instance) {
		service := response[instance.ServiceName]
		response[instance.ServiceName] = append(service, instance.Name)
	}
	err = iter.Err()
	if err != nil {
		return err
	}
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	n, err := w.Write(body)
	if n != len(body) {
		return &errors.Http{Code: http.StatusInternalServerError, Message: "Failed to write the response body."}
	}
	return err
}
