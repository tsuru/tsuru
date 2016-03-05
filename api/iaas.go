// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/permission"
	"gopkg.in/mgo.v2"
)

func machinesList(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	machines, err := iaas.ListMachines()
	if err != nil {
		return err
	}
	contexts := permission.ContextsForPermission(token, permission.PermMachineRead)
	allowedIaaS := map[string]struct{}{}
	for _, c := range contexts {
		if c.CtxType == permission.CtxGlobal {
			allowedIaaS = nil
			break
		}
		if c.CtxType == permission.CtxIaaS {
			allowedIaaS[c.Value] = struct{}{}
		}
	}
	for i := 0; allowedIaaS != nil && i < len(machines); i++ {
		if _, ok := allowedIaaS[machines[i].Iaas]; !ok {
			machines = append(machines[:i], machines[i+1:]...)
			i--
		}
	}
	err = json.NewEncoder(w).Encode(machines)
	if err != nil {
		return err
	}
	return nil
}

func machineDestroy(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	machineId := r.URL.Query().Get(":machine_id")
	if machineId == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "machine id is required"}
	}
	m, err := iaas.FindMachineById(machineId)
	if err != nil {
		if err == mgo.ErrNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: "machine not found"}
		}
		return err
	}
	allowed := permission.Check(token, permission.PermMachineDelete,
		permission.Context(permission.CtxIaaS, m.Iaas),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	return m.Destroy()
}

func templatesList(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	templates, err := iaas.ListTemplates()
	if err != nil {
		return err
	}
	contexts := permission.ContextsForPermission(token, permission.PermMachineTemplateRead)
	allowedIaaS := map[string]struct{}{}
	for _, c := range contexts {
		if c.CtxType == permission.CtxGlobal {
			allowedIaaS = nil
			break
		}
		if c.CtxType == permission.CtxIaaS {
			allowedIaaS[c.Value] = struct{}{}
		}
	}
	for i := 0; allowedIaaS != nil && i < len(templates); i++ {
		if _, ok := allowedIaaS[templates[i].IaaSName]; !ok {
			templates = append(templates[:i], templates[i+1:]...)
			i--
		}
	}
	return json.NewEncoder(w).Encode(templates)
}

func templateCreate(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	var paramTemplate iaas.Template
	err := json.NewDecoder(r.Body).Decode(&paramTemplate)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	allowed := permission.Check(token, permission.PermMachineTemplateCreate,
		permission.Context(permission.CtxIaaS, paramTemplate.IaaSName),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err = paramTemplate.Save()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

func templateDestroy(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	templateName := r.URL.Query().Get(":template_name")
	t, err := iaas.FindTemplate(templateName)
	if err != nil {
		if err == mgo.ErrNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: "template not found"}
		}
		return err
	}
	allowed := permission.Check(token, permission.PermMachineTemplateDelete,
		permission.Context(permission.CtxIaaS, t.IaaSName),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err = iaas.DestroyTemplate(templateName)
	if err != nil {
		return err
	}
	return nil
}

func templateUpdate(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	var paramTemplate iaas.Template
	err := json.NewDecoder(r.Body).Decode(&paramTemplate)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	templateName := r.URL.Query().Get(":template_name")
	dbTpl, err := iaas.FindTemplate(templateName)
	if err != nil {
		if err == mgo.ErrNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: "template not found"}
		}
		return err
	}
	allowed := permission.Check(token, permission.PermMachineTemplateUpdate,
		permission.Context(permission.CtxIaaS, dbTpl.IaaSName),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	return dbTpl.Update(&paramTemplate)
}
