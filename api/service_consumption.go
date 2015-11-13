// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/rec"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/mgo.v2/bson"
)

func createServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var body map[string]string
	err = json.Unmarshal(b, &body)
	if err != nil {
		return err
	}
	serviceName := body["service_name"]
	user, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(user.Email, "create-service-instance", string(b))
	srv, err := getServiceOrError(serviceName, user)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	instance := service.ServiceInstance{
		Name:      body["name"],
		PlanName:  body["plan"],
		TeamOwner: body["owner"],
	}
	return service.CreateServiceInstance(instance, &srv, user)
}

func removeServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	unbindAll := r.URL.Query().Get("unbindall")
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	rec.Log(u.Email, "remove-service-instance", instanceName)
	si, err := getServiceInstanceOrError(serviceName, instanceName, u)
	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	if err != nil {
		writer.Encode(io.SimpleJsonMessage{Error: err.Error()})
		return nil
	}
	if unbindAll == "true" {
		if len(si.Apps) > 0 {
			for _, appName := range si.Apps {
				_, app, instErr := getServiceInstance(si.ServiceName, si.Name, appName, u)
				if instErr != nil {
					writer.Encode(io.SimpleJsonMessage{Error: instErr.Error()})
					return nil
				}
				fmt.Fprintf(writer, "Unbind app %q ...\n", app.GetName())
				instErr = si.UnbindApp(app, true, writer)
				if instErr != nil {
					writer.Encode(io.SimpleJsonMessage{Error: instErr.Error()})
					return nil
				}
				fmt.Fprintf(writer, "\nInstance %q is not bound to the app %q anymore.\n", si.Name, app.GetName())
			}
			si, err = getServiceInstanceOrError(serviceName, instanceName, u)
			if err != nil {
				writer.Encode(io.SimpleJsonMessage{Error: err.Error()})
				return nil
			}
		}
	}
	err = service.DeleteInstance(si)
	if err != nil {
		var msg string
		if err == service.ErrServiceInstanceBound {
			msg = strings.Join(si.Apps, ",")
		}
		writer.Encode(io.SimpleJsonMessage{Message: msg, Error: err.Error()})
		return nil
	}
	writer.Write([]byte("service instance successfuly removed"))
	return nil
}

func serviceInstances(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get("app")
	rec.Log(u.Email, "list-service-instances", "app="+appName)
	services, _ := service.GetServicesByTeamKindAndNoRestriction("teams", u)
	sInstances, _ := service.GetServiceInstancesByServicesAndTeams(services, u, appName)
	result := make([]service.ServiceModel, len(services))
	for i, s := range services {
		result[i].Service = s.Name
		result[i].Instances = []string{}
		for _, si := range sInstances {
			if si.ServiceName == s.Name {
				result[i].Instances = append(result[i].Instances, si.Name)
			}
		}
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	n, err := w.Write(body)
	if n != len(body) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write the response body."}
	}
	return err
}

func serviceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	sName := r.URL.Query().Get(":service")
	siName := r.URL.Query().Get(":instance")
	instance, err := getServiceInstanceOrError(sName, siName, u)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(instance)
}

func serviceInstanceStatus(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	siName := r.URL.Query().Get(":instance")
	sName := r.URL.Query().Get(":service")
	si, err := getServiceInstanceOrError(sName, siName, u)
	if err != nil {
		return err
	}
	rec.Log(u.Email, "service-instance-status", siName)
	var b string
	if b, err = si.Status(); err != nil {
		msg := fmt.Sprintf("Could not retrieve status of service instance, error: %s", err)
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
	}
	b = fmt.Sprintf(`Service instance "%s" is %s`, siName, b)
	n, err := w.Write([]byte(b))
	if n != len(b) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write response body"}
	}
	return nil
}

func serviceInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	serviceName := r.URL.Query().Get(":name")
	rec.Log(u.Email, "service-info", serviceName)
	_, err = getServiceOrError(serviceName, u)
	if err != nil {
		return err
	}
	instances := []service.ServiceInstance{}
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	teamsNames := auth.GetTeamsNames(teams)
	q := bson.M{"service_name": serviceName, "teams": bson.M{"$in": teamsNames}}
	err = conn.ServiceInstances().Find(q).All(&instances)
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

func serviceDoc(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	sName := r.URL.Query().Get(":name")
	rec.Log(u.Email, "service-doc", sName)
	s, err := getServiceOrError(sName, u)
	if err != nil {
		return err
	}
	w.Write([]byte(s.Doc))
	return nil
}

func getServiceOrError(name string, u *auth.User) (service.Service, error) {
	s := service.Service{Name: name}
	err := s.Get()
	if err != nil {
		return s, &errors.HTTP{Code: http.StatusNotFound, Message: "Service not found"}
	}
	if !s.IsRestricted {
		return s, nil
	}
	if !auth.CheckUserAccess(s.Teams, u) {
		msg := "This user does not have access to this service"
		return s, &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	return s, err
}

func getServiceInstanceOrError(serviceName string, instanceName string, u *auth.User) (*service.ServiceInstance, error) {
	si, err := service.GetServiceInstance(serviceName, instanceName, u)
	if err != nil {
		switch err {
		case service.ErrServiceInstanceNotFound:
			return nil, &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		case service.ErrAccessNotAllowed:
			return nil, &errors.HTTP{
				Code:    http.StatusForbidden,
				Message: err.Error(),
			}
		default:
			return nil, err
		}
	}
	return si, nil
}

func servicePlans(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	serviceName := r.URL.Query().Get(":name")
	rec.Log(u.Email, "service-plans", serviceName)
	plans, err := service.GetPlansByServiceName(serviceName)
	if err != nil {
		return err
	}
	b, err := json.Marshal(plans)
	if err != nil {
		return nil
	}
	w.Write(b)
	return nil
}

func serviceInstanceProxy(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	sName := r.URL.Query().Get(":service")
	siName := r.URL.Query().Get(":instance")
	si, err := getServiceInstanceOrError(sName, siName, u)
	if err != nil {
		return err
	}
	path := r.URL.Query().Get("callback")
	rec.Log(u.Email, "service-instance-proxy", siName, path)
	return service.Proxy(si.Service(), path, w, r)
}

func serviceInstanceGrantTeam(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	siName := r.URL.Query().Get(":instance")
	sName := r.URL.Query().Get(":service")
	si, err := getServiceInstanceOrError(sName, siName, u)
	if err != nil {
		return err
	}
	teamName := r.URL.Query().Get(":team")
	rec.Log(u.Email, "service-grant-team", siName, teamName)
	return si.Grant(teamName)
}

func serviceInstanceRevokeTeam(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	siName := r.URL.Query().Get(":instance")
	sName := r.URL.Query().Get(":service")
	si, err := getServiceInstanceOrError(sName, siName, u)
	if err != nil {
		return err
	}
	teamName := r.URL.Query().Get(":team")
	rec.Log(u.Email, "service-revoke-team", siName, teamName)
	return si.Revoke(teamName)
}
