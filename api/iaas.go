// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/iaas"
	"gopkg.in/mgo.v2"
)

func machinesList(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	machines, err := iaas.ListMachines()
	if err != nil {
		return err
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
	return m.Destroy()
}

func templatesList(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	templates, err := iaas.ListTemplates()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(templates)
}

func templateCreate(w http.ResponseWriter, r *http.Request, token auth.Token) error {
	var paramTemplate iaas.Template
	err := json.NewDecoder(r.Body).Decode(&paramTemplate)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
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
	err := iaas.DestroyTemplate(templateName)
	if err != nil {
		return err
	}
	return nil
}
