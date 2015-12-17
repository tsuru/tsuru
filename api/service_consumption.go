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
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/rec"
	"github.com/tsuru/tsuru/service"
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
	srv, err := getService(serviceName)
	if err != nil {
		return err
	}
	instance := service.ServiceInstance{
		Name:      body["name"],
		PlanName:  body["plan"],
		TeamOwner: body["owner"],
	}
	if instance.TeamOwner == "" {
		teamOwner, err := permission.TeamForPermission(t, permission.PermServiceInstanceCreate)
		if err != nil {
			return err
		}
		instance.TeamOwner = teamOwner
	}
	allowed := permission.Check(t, permission.PermServiceInstanceCreate,
		permission.Context(permission.CtxTeam, instance.TeamOwner),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	if srv.IsRestricted {
		allowed := permission.Check(t, permission.PermServiceRead,
			append(permission.Contexts(permission.CtxTeam, srv.Teams),
				permission.Context(permission.CtxService, srv.Name))...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	rec.Log(user.Email, "create-service-instance", string(b))
	return service.CreateServiceInstance(instance, &srv, user)
}

func removeServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	unbindAll := r.URL.Query().Get("unbindall")
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	permissionValue := serviceName + "/" + instanceName

	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		writer.Encode(io.SimpleJsonMessage{Error: err.Error()})
		return nil
	}
	allowed := permission.Check(t, permission.PermServiceInstanceDelete,
		append(permission.Contexts(permission.CtxTeam, serviceInstance.Teams),
			permission.Context(permission.CtxServiceInstance, permissionValue),
		)...,
	)
	if !allowed {
		writer.Encode(io.SimpleJsonMessage{Error: permission.ErrUnauthorized.Error()})
		return nil
	}
	rec.Log(t.GetUserName(), "remove-service-instance", serviceName, instanceName)
	if unbindAll == "true" {
		if len(serviceInstance.Apps) > 0 {
			for _, appName := range serviceInstance.Apps {
				_, app, instErr := getServiceInstance(serviceInstance.ServiceName, serviceInstance.Name, appName)
				if instErr != nil {
					writer.Encode(io.SimpleJsonMessage{Error: instErr.Error()})
					return nil
				}
				fmt.Fprintf(writer, "Unbind app %q ...\n", app.GetName())
				instErr = serviceInstance.UnbindApp(app, true, writer)
				if instErr != nil {
					writer.Encode(io.SimpleJsonMessage{Error: instErr.Error()})
					return nil
				}
				fmt.Fprintf(writer, "\nInstance %q is not bound to the app %q anymore.\n", serviceInstance.Name, app.GetName())
			}
			serviceInstance, err = getServiceInstanceOrError(serviceName, instanceName)
			if err != nil {
				writer.Encode(io.SimpleJsonMessage{Error: err.Error()})
				return nil
			}
		}
	}
	err = service.DeleteInstance(serviceInstance)
	if err != nil {
		var msg string
		if err == service.ErrServiceInstanceBound {
			msg = strings.Join(serviceInstance.Apps, ",")
		}
		writer.Encode(io.SimpleJsonMessage{Message: msg, Error: err.Error()})
		return nil
	}
	writer.Write([]byte("service instance successfuly removed"))
	return nil
}

func readableInstances(t auth.Token, appName, serviceName string) ([]service.ServiceInstance, error) {
	teams := []string{}
	instanceNames := []string{}
	contexts := permission.ContextsForPermission(t, permission.PermServiceInstanceRead)
	for _, c := range contexts {
		if c.CtxType == permission.CtxGlobal {
			teams = nil
			instanceNames = nil
			break
		}
		switch c.CtxType {
		case permission.CtxServiceInstance:
			parts := strings.SplitAfterN(c.Value, "/", 2)
			if len(parts) == 2 && parts[0] == serviceName {
				instanceNames = append(instanceNames, parts[1])
			}
		case permission.CtxTeam:
			teams = append(teams, c.Value)
		}
	}
	return service.GetServicesInstancesByTeamsAndNames(teams, instanceNames, appName, serviceName)
}

func readableServices(t auth.Token) ([]service.Service, error) {
	teams := []string{}
	serviceNames := []string{}
	contexts := permission.ContextsForPermission(t, permission.PermServiceRead)
	for _, c := range contexts {
		if c.CtxType == permission.CtxGlobal {
			teams = nil
			serviceNames = nil
			break
		}
		switch c.CtxType {
		case permission.CtxService:
			serviceNames = append(serviceNames, c.Value)
		case permission.CtxTeam:
			teams = append(teams, c.Value)
		}
	}
	return service.GetServicesByTeamsAndServices(teams, serviceNames)
}

func serviceInstances(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get("app")
	rec.Log(t.GetUserName(), "list-service-instances", "app="+appName)
	instances, err := readableInstances(t, appName, "")
	if err != nil {
		return err
	}
	services, err := readableServices(t)
	if err != nil {
		return err
	}
	servicesMap := map[string]*service.ServiceModel{}
	for _, s := range services {
		if _, in := servicesMap[s.Name]; !in {
			servicesMap[s.Name] = &service.ServiceModel{
				Service:   s.Name,
				Instances: []string{},
			}
		}
	}
	for _, instance := range instances {
		entry := servicesMap[instance.ServiceName]
		if entry == nil {
			continue
		}
		entry.Instances = append(entry.Instances, instance.Name)
		entry.Plans = append(entry.Plans, instance.PlanName)
	}
	result := []service.ServiceModel{}
	for _, entry := range servicesMap {
		result = append(result, *entry)
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
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	permissionValue := serviceName + "/" + instanceName
	instance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceRead,
		append(permission.Contexts(permission.CtxTeam, instance.Teams),
			permission.Context(permission.CtxServiceInstance, permissionValue),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	return json.NewEncoder(w).Encode(instance)
}

func serviceInstanceStatus(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	permissionValue := serviceName + "/" + instanceName
	allowed := permission.Check(t, permission.PermServiceInstanceReadStatus,
		append(permission.Contexts(permission.CtxTeam, serviceInstance.Teams),
			permission.Context(permission.CtxServiceInstance, permissionValue),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(t.GetUserName(), "service-instance-status", serviceName, instanceName)
	var b string
	if b, err = serviceInstance.Status(); err != nil {
		msg := fmt.Sprintf("Could not retrieve status of service instance, error: %s", err)
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
	}
	b = fmt.Sprintf(`Service instance "%s" is %s`, instanceName, b)
	n, err := w.Write([]byte(b))
	if n != len(b) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write response body"}
	}
	return nil
}

func serviceInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":name")
	_, err := getService(serviceName)
	if err != nil {
		return err
	}
	rec.Log(t.GetUserName(), "service-info", serviceName)
	instances, err := readableInstances(t, "", serviceName)
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
	serviceName := r.URL.Query().Get(":name")
	rec.Log(t.GetUserName(), "service-doc", serviceName)
	s, err := getService(serviceName)
	if err != nil {
		return err
	}
	if s.IsRestricted {
		allowed := permission.Check(t, permission.PermServiceReadDoc,
			append(permission.Contexts(permission.CtxTeam, s.Teams),
				permission.Context(permission.CtxService, s.Name),
			)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	w.Write([]byte(s.Doc))
	return nil
}

func getServiceInstanceOrError(serviceName string, instanceName string) (*service.ServiceInstance, error) {
	serviceInstance, err := service.GetServiceInstance(serviceName, instanceName)
	if err != nil {
		switch err {
		case service.ErrServiceInstanceNotFound:
			return nil, &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		default:
			return nil, err
		}
	}
	return serviceInstance, nil
}

func servicePlans(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":name")
	s, err := getService(serviceName)
	if err != nil {
		return err
	}
	if s.IsRestricted {
		allowed := permission.Check(t, permission.PermServiceReadPlans,
			append(permission.Contexts(permission.CtxTeam, s.Teams),
				permission.Context(permission.CtxService, s.Name),
			)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	rec.Log(t.GetUserName(), "service-plans", serviceName)
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
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	permissionValue := serviceName + "/" + instanceName
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateProxy,
		append(permission.Contexts(permission.CtxTeam, serviceInstance.Teams),
			permission.Context(permission.CtxServiceInstance, permissionValue),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	path := r.URL.Query().Get("callback")
	rec.Log(t.GetUserName(), "service-instance-proxy", serviceName, instanceName, path)
	return service.Proxy(serviceInstance.Service(), path, w, r)
}

func serviceInstanceGrantTeam(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	permissionValue := serviceName + "/" + instanceName
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateGrant,
		append(permission.Contexts(permission.CtxTeam, serviceInstance.Teams),
			permission.Context(permission.CtxServiceInstance, permissionValue),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	teamName := r.URL.Query().Get(":team")
	rec.Log(t.GetUserName(), "service-grant-team", serviceName, instanceName, teamName)
	return serviceInstance.Grant(teamName)
}

func serviceInstanceRevokeTeam(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	permissionValue := serviceName + "/" + instanceName
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateRevoke,
		append(permission.Contexts(permission.CtxTeam, serviceInstance.Teams),
			permission.Context(permission.CtxServiceInstance, permissionValue),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	teamName := r.URL.Query().Get(":team")
	rec.Log(t.GetUserName(), "service-revoke-team", serviceName, instanceName, teamName)
	return serviceInstance.Revoke(teamName)
}
