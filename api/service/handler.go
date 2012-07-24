package service

import (
	"encoding/json"
	"fmt"
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
	s, err := getServiceOrError(yaml.Id, u)
	if err != nil {
		return err
	}
	s.Endpoint = yaml.Endpoint
	s.Bootstrap = yaml.Bootstrap
	return s.update()
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
	err := db.Session.Services().Find(bson.M{"_id": sJson["service_name"], "status": bson.M{"$ne": "deleted"}}).One(&s)
	if err != nil {
		msg := err.Error()
		if msg == "not found" {
			msg = fmt.Sprintf("Service %s does not exist.", sJson["service_name"])
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

func getServiceOrError(name string, u *auth.User) (Service, error) {
	s := Service{Name: name}
	err := s.Get()
	if err != nil {
		return s, &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !auth.CheckUserAccess(s.Teams, u) {
		msg := "This user does not have access to this service"
		return s, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	return s, err
}

func DeleteHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	s, err := getServiceOrError(r.URL.Query().Get(":name"), u)
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

func ServicesInstancesHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	response := serviceAndServiceInstancesByTeams("teams", u)
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

type ServiceModel struct {
	Service   string
	Instances []string
}

func ServicesHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	results := serviceAndServiceInstancesByTeams("owner_teams", u)
	b, err := json.Marshal(results)
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	n, err := w.Write(b)
	if n != len(b) {
		return &errors.Http{Code: http.StatusInternalServerError, Message: "Failed to write response body"}
	}
	return err
}

func serviceAndServiceInstancesByTeams(teamKind string, u *auth.User) []ServiceModel {
	var teams []auth.Team
	q := bson.M{"users.email": u.Email}
	db.Session.Teams().Find(q).Select(bson.M{"name": 1}).All(&teams)
	var services []Service
	q = bson.M{teamKind: bson.M{"$in": auth.GetTeamsNames(teams)}}
	db.Session.Services().Find(q).Select(bson.M{"name": 1}).All(&services)
	var sInsts []ServiceInstance
	q = bson.M{"service_name": bson.M{"$in": GetServicesNames(services)}}
	db.Session.ServiceInstances().Find(q).Select(bson.M{"name": 1, "service_name": 1}).All(&sInsts)
	results := make([]ServiceModel, len(services))
	for i, s := range services {
		results[i].Service = s.Name
		for _, si := range sInsts {
			if si.ServiceName == s.Name {
				results[i].Instances = append(results[i].Instances, si.Name)
			}
		}
	}
	return results
}

func ServiceInstanceStatusHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	siName := r.URL.Query().Get(":instance")
	var si ServiceInstance
	if siName == "" {
		return &errors.Http{Code: http.StatusBadRequest, Message: "Service instance name not provided."}
	}
	err := db.Session.ServiceInstances().Find(bson.M{"_id": siName}).One(&si)
	if err != nil {
		msg := fmt.Sprintf("Service instance does not exists, error: %s", err.Error())
		return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	s := si.Service()
	var b string
	if cli, err := s.GetClient("production"); err == nil {
		if b, err = cli.Status(&si); err != nil {
			msg := fmt.Sprintf("Could not retrieve status of service instance, error: %s", err.Error())
			return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
		}
	} else {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	b = fmt.Sprintf(`Service instance "%s" is %s`, siName, b)
	n, err := w.Write([]byte(b))
	if n != len(b) {
		return &errors.Http{Code: http.StatusInternalServerError, Message: "Failed to write response body"}
	}
	return nil
}

func ServiceInfoHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	serviceName := r.URL.Query().Get(":name")
	s := Service{Name: serviceName}
	err := s.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !auth.CheckUserAccess(s.Teams, u) {
		msg := "This user does not have access to this service"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	instances := []ServiceInstance{}
	err = db.Session.ServiceInstances().Find(bson.M{"service_name": serviceName}).All(&instances)
	if err != nil {
		return err
	}
	b, err := json.Marshal(instances)
	if err != nil {
		return nil
	}
	w.Write(b)
	return nil
}
