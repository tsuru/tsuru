// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
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

// title: service instance create
// path: /services/{service}/instances
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   201: Service created
//   400: Invalid data
//   401: Unauthorized
//   409: Service already exists
func createServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":service")
	user, err := t.User()
	if err != nil {
		return err
	}
	srv, err := getService(serviceName)
	if err != nil {
		return err
	}
	instance := service.ServiceInstance{
		Name:        r.FormValue("name"),
		PlanName:    r.FormValue("plan"),
		TeamOwner:   r.FormValue("owner"),
		Description: r.FormValue("description"),
	}
	var teamOwner string
	if instance.TeamOwner == "" {
		teamOwner, err = permission.TeamForPermission(t, permission.PermServiceInstanceCreate)
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
	rec.Log(user.Email, "create-service-instance", fmt.Sprintf("%#v", instance))
	err = service.CreateServiceInstance(instance, &srv, user)
	if err == service.ErrInstanceNameAlreadyExists {
		return &errors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	if err == service.ErrInvalidInstanceName {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

// title: service instance update
// path: /services/{service}/instances/{instance}
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Service instance updated
//   400: Invalid data
//   401: Unauthorized
//   404: Service instance not found
func updateServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	description := r.FormValue("description")
	if description == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid value for description",
		}
	}
	si, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateDescription,
		permission.Context(permission.CtxServiceInstance, si.Name),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	user, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(user.Email, "update-service-instance", "description="+description)
	si.Description = description
	return service.UpdateService(si)
}

// title: remove service instance
// path: /services/{name}/instances/{instance}
// method: DELETE
// produce: application/x-json-stream
// responses:
//   200: Service removed
//   401: Unauthorized
//   404: Service instance not found
func removeServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	unbindAll := r.URL.Query().Get("unbindall")
	serviceName := r.URL.Query().Get(":service")
	instanceName := r.URL.Query().Get(":instance")
	permissionValue := serviceName + "/" + instanceName
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	w.Header().Set("Content-Type", "application/x-json-stream")
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

// title: service instance list
// path: /services/instances
// method: GET
// produce: application/json
// responses:
//   200: List services instances
//   204: No content
//   401: Unauthorized
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
	if len(result) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	n, err := w.Write(body)
	if n != len(body) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write the response body."}
	}
	w.Header().Set("Content-Type", "application/json")
	return err
}

// title: service instance status
// path: /services/{service}/instances/{instance}/status
// method: GET
// responses:
//   200: List services instances
//   401: Unauthorized
//   404: Service instance not found
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

type serviceInstanceInfo struct {
	Apps            []string
	Teams           []string
	TeamOwner       string
	Description     string
	PlanName        string
	PlanDescription string
	CustomInfo      map[string]string
}

// title: service instance info
// path: /services/{service}/instances/{instance}
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Service instance not found
func serviceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName := r.URL.Query().Get(":instance")
	serviceName := r.URL.Query().Get(":service")
	serviceInstance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return err
	}
	permissionValue := serviceName + "/" + instanceName
	allowed := permission.Check(t, permission.PermServiceInstanceRead,
		append(permission.Contexts(permission.CtxTeam, serviceInstance.Teams),
			permission.Context(permission.CtxServiceInstance, permissionValue),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(t.GetUserName(), "service-instance-info", serviceName, instanceName)
	info, err := serviceInstance.Info()
	if err != nil {
		return err
	}
	plan, err := service.GetPlanByServiceNameAndPlanName(serviceName, serviceInstance.PlanName)
	if err != nil {
		return err
	}
	sInfo := serviceInstanceInfo{
		Apps:            serviceInstance.Apps,
		Teams:           serviceInstance.Teams,
		TeamOwner:       serviceInstance.TeamOwner,
		Description:     serviceInstance.Description,
		PlanName:        plan.Name,
		PlanDescription: plan.Description,
		CustomInfo:      info,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(sInfo)
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
	return json.NewEncoder(w).Encode(instances)
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

// title: service plans
// path: /services/{name}/plans
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Service not found
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
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(plans)
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

// title: grant access to service instance
// path: /services/{service}/instances/permission/{instance}/{team}
// consume: application/x-www-form-urlencoded
// method: PUT
// responses:
//   200: Access revoked
//   401: Unauthorized
//   404: Service instance not found
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

// title: revoke access to service instance
// path: /services/{service}/instances/permission/{instance}/{team}
// method: DELETE
// responses:
//   200: Access revoked
//   401: Unauthorized
//   404: Service instance not found
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
