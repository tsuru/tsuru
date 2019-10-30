// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/globalsign/mgo"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/permission"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

// title: machine list
// path: /iaas/machines
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
func machinesList(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	machines, err := iaas.ListMachines()
	if err != nil {
		return err
	}
	contexts := permission.ContextsForPermission(token, permission.PermMachineRead)
	allowedIaaS := map[string]struct{}{}
	for _, c := range contexts {
		if c.CtxType == permTypes.CtxGlobal {
			allowedIaaS = nil
			break
		}
		if c.CtxType == permTypes.CtxIaaS {
			allowedIaaS[c.Value] = struct{}{}
		}
	}
	for i := 0; allowedIaaS != nil && i < len(machines); i++ {
		if _, ok := allowedIaaS[machines[i].Iaas]; !ok {
			machines = append(machines[:i], machines[i+1:]...)
			i--
		}
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(machines)
}

// title: machine destroy
// path: /iaas/machines/{machine_id}
// method: DELETE
// responses:
//   200: OK
//   400: Invalid data
//   401: Unauthorized
//   404: Not found
func machineDestroy(w http.ResponseWriter, r *http.Request, token auth.Token) (err error) {
	machineID := r.URL.Query().Get(":machine_id")
	if machineID == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "machine id is required"}
	}
	force, _ := strconv.ParseBool(r.URL.Query().Get("force"))
	m, err := iaas.FindMachineById(machineID)
	if err != nil {
		if err == iaas.ErrMachineNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: "machine not found"}
		}
		return err
	}
	iaasCtx := permission.Context(permTypes.CtxIaaS, m.Iaas)
	allowed := permission.Check(token, permission.PermMachineDelete, iaasCtx)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeIaas, Value: m.Iaas},
		Kind:       permission.PermMachineDelete,
		Owner:      token,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermMachineReadEvents, iaasCtx),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return m.Destroy(iaas.DestroyParams{
		Force: force,
	})
}

// title: machine template list
// path: /iaas/templates
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
func templatesList(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	templates, err := iaas.ListTemplates()
	if err != nil {
		return err
	}
	contexts := permission.ContextsForPermission(token, permission.PermMachineTemplateRead)
	allowedIaaS := map[string]struct{}{}
	for _, c := range contexts {
		if c.CtxType == permTypes.CtxGlobal {
			allowedIaaS = nil
			break
		}
		if c.CtxType == permTypes.CtxIaaS {
			allowedIaaS[c.Value] = struct{}{}
		}
	}
	for i := 0; allowedIaaS != nil && i < len(templates); i++ {
		if _, ok := allowedIaaS[templates[i].IaaSName]; !ok {
			templates = append(templates[:i], templates[i+1:]...)
			i--
		}
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(templates)
}

// title: template create
// path: /iaas/templates
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   201: Template created
//   400: Invalid data
//   401: Unauthorized
//   409: Existent template
func templateCreate(w http.ResponseWriter, r *http.Request, token auth.Token) (err error) {
	var paramTemplate iaas.Template
	err = ParseInput(r, &paramTemplate)
	if err != nil {
		return err
	}
	iaasCtx := permission.Context(permTypes.CtxIaaS, paramTemplate.IaaSName)
	allowed := permission.Check(token, permission.PermMachineTemplateCreate, iaasCtx)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeIaas, Value: paramTemplate.IaaSName},
		Kind:       permission.PermMachineTemplateCreate,
		Owner:      token,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermMachineReadEvents, iaasCtx),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	_, err = iaas.FindTemplate(paramTemplate.Name)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	if err == nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: fmt.Sprintf("template name \"%s\" already used", paramTemplate.Name)}
	}
	err = paramTemplate.Save()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

// title: template destroy
// path: /iaas/templates/{template_name}
// method: DELETE
// responses:
//   200: OK
//   401: Unauthorized
//   404: Not found
func templateDestroy(w http.ResponseWriter, r *http.Request, token auth.Token) (err error) {
	templateName := r.URL.Query().Get(":template_name")
	t, err := iaas.FindTemplate(templateName)
	if err != nil {
		if err == mgo.ErrNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: "template not found"}
		}
		return err
	}
	iaasCtx := permission.Context(permTypes.CtxIaaS, t.IaaSName)
	allowed := permission.Check(token, permission.PermMachineTemplateDelete, iaasCtx)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeIaas, Value: t.IaaSName},
		Kind:       permission.PermMachineTemplateDelete,
		Owner:      token,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermMachineReadEvents, iaasCtx),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return iaas.DestroyTemplate(templateName)
}

// title: template update
// path: /iaas/templates/{template_name}
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: OK
//   400: Invalid data
//   401: Unauthorized
//   404: Not found
func templateUpdate(w http.ResponseWriter, r *http.Request, token auth.Token) (err error) {
	var paramTemplate iaas.Template
	err = ParseInput(r, &paramTemplate)
	if err != nil {
		return err
	}
	templateName := r.URL.Query().Get(":template_name")
	dbTpl, err := iaas.FindTemplate(templateName)
	if err != nil {
		if err == mgo.ErrNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: "template not found"}
		}
		return err
	}
	iaasValue := InputValue(r, "IaaSName")
	if iaasValue != "" {
		dbTpl.IaaSName = iaasValue
	}
	iaasCtx := permission.Context(permTypes.CtxIaaS, dbTpl.IaaSName)
	allowed := permission.Check(token, permission.PermMachineTemplateUpdate, iaasCtx)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeIaas, Value: dbTpl.IaaSName},
		Kind:       permission.PermMachineTemplateUpdate,
		Owner:      token,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermMachineReadEvents, iaasCtx),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return dbTpl.Update(&paramTemplate)
}
