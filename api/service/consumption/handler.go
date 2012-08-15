package consumption

import (
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/log"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
)

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
	var s service.Service
	err = validateInstanceForCreation(&s, sJson, u)
	if err != nil {
		log.Print("Got error while validation:")
		log.Print(err.Error())
		return err
	}
	var teamNames []string
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	for _, t := range teams {
		if s.HasTeam(&t) || !s.IsRestricted {
			teamNames = append(teamNames, t.Name)
		}
	}
	si := service.ServiceInstance{
		Name:        sJson["name"],
		ServiceName: sJson["service_name"],
		Teams:       teamNames,
	}
	go func() {
		if cli, err := s.GetClient("production"); err == nil {
			if cli.Create(&si) != nil {
				log.Print("Error while calling create action from service api.")
				log.Print(err.Error())
			}
		}
	}()
	err = si.Create()
	if err != nil {
		return err
	}
	fmt.Fprint(w, "success")
	return nil
}

func validateInstanceForCreation(s *service.Service, sJson map[string]string, u *auth.User) error {
	err := db.Session.Services().Find(bson.M{"_id": sJson["service_name"], "status": bson.M{"$ne": "deleted"}}).One(&s)
	if err != nil {
		msg := err.Error()
		if msg == "not found" {
			msg = fmt.Sprintf("Service %s does not exist.", sJson["service_name"])
		}
		return &errors.Http{Code: http.StatusNotFound, Message: msg}
	}
	_, err = GetServiceOrError(sJson["service_name"], u)
	if err != nil {
		return err
	}
	return nil
}

func RemoveServiceInstanceHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	name := r.URL.Query().Get(":name")
	si, err := GetServiceInstanceOrError(name, u)
	if err != nil {
		return err
	}
	if len(si.Apps) > 0 {
		msg := "This service instance has binded apps. Unbind them before removing it"
		return &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	err = db.Session.ServiceInstances().Remove(bson.M{"_id": name})
	if err != nil {
		return err
	}
	w.Write([]byte("service instance successfuly removed"))
	return nil
}

func ServicesInstancesHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	response := ServiceAndServiceInstancesByTeams(u)
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

func ServiceInstanceStatusHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	// #TODO (flaviamissi) should check if user has access to service
	// just call GetServiceInstanceOrError should be enough
	siName := r.URL.Query().Get(":instance")
	var si service.ServiceInstance
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
	_, err := GetServiceOrError(serviceName, u)
	if err != nil {
		return err
	}
	instances := []service.ServiceInstance{}
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

func Doc(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	sName := r.URL.Query().Get(":name")
	s, err := GetServiceOrError(sName, u)
	if err != nil {
		return err
	}
	w.Write([]byte(s.Doc))
	return nil
}
