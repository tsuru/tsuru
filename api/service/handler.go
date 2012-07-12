package service

import (
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/ec2"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/log"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	"net/http"
	"time"
)

type serviceYaml struct {
	Id        string
	Endpoint  map[string]string
	Bootstrap map[string]string
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
	err = s.Create()
	if err != nil {
		return err
	}
	fmt.Fprint(w, "success")
	return nil
}

func CreateInstanceHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	log.Print("Receiving request to create a service instance")
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Print("Got error while reading request body:")
		log.Print(err.Error())
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	var sJson map[string]string
	err = json.Unmarshal(b, &sJson)
	if err != nil {
		log.Print("Got a problem while unmarshalling request's json:")
		log.Print(err.Error())
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	var s Service
	err = validateForInstanceCreation(&s, sJson, u)
	if err != nil {
		log.Print("Got error while validation:")
		log.Print(err.Error())
		return err
	}
	instance := &ec2.Instance{}
	if s.Bootstrap["when"] == OnNewInstance {
		instance, err = ec2.RunInstance(s.Bootstrap["ami"], "") //missing user data
		if err != nil {
			msg := fmt.Sprintf("Instance for service could not be created. \nError: %s", err.Error())
			return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
		}
	}
	var teamNames []string
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	for _, t := range teams {
		if s.hasTeam(&t) {
			teamNames = append(teamNames, t.Name)
		}
	}
	si := ServiceInstance{
		Name:        sJson["name"],
		ServiceName: sJson["service_name"],
		Instance:    instance.Id,
		Teams:       teamNames,
	}
	go callServiceApi(s, si)
	err = si.Create()
	if err != nil {
		return err
	}
	fmt.Fprint(w, "success")
	return nil
}

func callServiceApi(s Service, si ServiceInstance) {
	checkInstanceState := func() bool {
		if when, ok := s.Bootstrap["when"]; !ok || when != OnNewInstance {
			return true
		}
		db.Session.ServiceInstances().Find(bson.M{"_id": si.Name}).One(&si)
		return si.Host != "" && si.State == "running"
	}
	ch := time.Tick(1e9)
	for _ = range ch {
		if checkInstanceState() {
			if cli, err := s.GetClient("production"); err == nil {
				si.Env, err = cli.Create(&si)
				if err != nil {
					log.Print("Error while calling create action from service api.")
					log.Print(err.Error())
				}
				si.State = "running"
				db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
			}
			break
		}
	}
}

func validateForInstanceCreation(s *Service, sJson map[string]string, u *auth.User) error {
	err := db.Session.Services().Find(bson.M{"_id": sJson["service_name"]}).One(&s)
	if err != nil {
		msg := err.Error()
		if msg == "not found" {
			msg = fmt.Sprintf("Service %s does not exists.", sJson["service_name"])
		}
		return &errors.Http{Code: http.StatusNotFound, Message: msg}
	}
	var teams []auth.Team
	err = db.Session.Teams().Find(bson.M{"users.email": u.Email}).All(&teams)
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	q := bson.M{"_id": sJson["service_name"], "teams": bson.M{"$in": auth.GetTeamsNames(teams)}}
	n, err := db.Session.Services().Find(q).Count()
	if n == 0 {
		msg := fmt.Sprintf("You don't have access to service %s", sJson["service_name"])
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	return nil
}

func DeleteHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	s := Service{Name: r.URL.Query().Get(":name")}
	err := s.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !auth.CheckUserAccess(s.Teams, u) {
		msg := "This user does not have access to this service"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	s.Delete()
	fmt.Fprint(w, "success")
	return nil
}

func serviceInstanceAndAppOrError(instanceName, appName string, u *auth.User) (instance ServiceInstance, a app.App, err error) {
	err = db.Session.ServiceInstances().Find(bson.M{"_id": instanceName}).One(&instance)
	if err != nil {
		err = &errors.Http{Code: http.StatusNotFound, Message: "Instance not found"}
		return
	}
	if !auth.CheckUserAccess(instance.Teams, u) {
		err = &errors.Http{Code: http.StatusForbidden, Message: "This user does not have access to this instance"}
		return
	}
	if instance.State != "running" {
		err = &errors.Http{Code: http.StatusPreconditionFailed, Message: "This service instance is not ready yet."}
		return
	}
	err = db.Session.Apps().Find(bson.M{"name": appName}).One(&a)
	if err != nil {
		err = &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
		return
	}
	if !auth.CheckUserAccess(a.Teams, u) {
		err = &errors.Http{Code: http.StatusForbidden, Message: "This user does not have access to this app"}
		return
	}
	return
}

func BindHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	instanceName, appName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app")
	instance, a, err := serviceInstanceAndAppOrError(instanceName, appName, u)
	if err != nil {
		return err
	}
	instance.Apps = append(instance.Apps, a.Name)
	err = db.Session.ServiceInstances().Update(bson.M{"_id": instanceName}, instance)
	if err != nil {
		return err
	}
	var envVars []app.EnvVar
	var setEnv = func(a app.App, env map[string]string) {
		for k, v := range env {
			envVars = append(envVars, app.EnvVar{
				Name:         k,
				Value:        v,
				Public:       false,
				InstanceName: instance.Name,
			})
		}
	}
	setEnv(a, instance.Env)
	err = db.Session.Apps().Update(bson.M{"name": appName}, a)
	if err != nil {
		return err
	}
	var cli *Client
	if cli, err = instance.Service().GetClient("production"); err == nil {
		if len(a.Units) == 0 {
			return &errors.Http{Code: http.StatusPreconditionFailed, Message: "This app does not have an IP yet."}
		}
		env, err := cli.Bind(&instance, &a)
		if err != nil {
			return err
		}
		setEnv(a, env)
	}
	return app.SetEnvsToApp(&a, envVars, false)
}

func UnbindHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	instanceName, appName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app")
	instance, a, err := serviceInstanceAndAppOrError(instanceName, appName, u)
	if err != nil {
		return err
	}
	instance.RemoveApp(a.Name)
	err = db.Session.ServiceInstances().Update(bson.M{"_id": instanceName}, instance)
	if err != nil {
		return err
	}
	var envVars []string
	for k, _ := range a.InstanceEnv(instance.Name) {
		envVars = append(envVars, k)
	}
	return app.UnsetEnvFromApp(&a, envVars, false)
}

func getServiceAndTeamOrError(serviceName string, teamName string, u *auth.User) (*Service, *auth.Team, error) {
	service := &Service{Name: serviceName}
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

func ServicesHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	teams, err := u.Teams()
	if err != nil {
		return err
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
	iter := db.Session.ServiceInstances().Find(bson.M{"teams": bson.M{"$in": teamNames}}).Iter()
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
