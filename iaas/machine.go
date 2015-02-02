// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new iaas on tsuru.
package iaas

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2/bson"
)

type Machine struct {
	Id             string `bson:"_id"`
	Iaas           string
	Status         string
	Address        string
	CreationParams map[string]string
}

func CreateMachine(params map[string]string) (*Machine, error) {
	templateName := params["template"]
	if templateName != "" {
		template, err := FindTemplate(templateName)
		if err != nil {
			return nil, err
		}
		templateParams := template.paramsMap()
		delete(params, "template")
		// User params will override template params
		for k, v := range templateParams {
			_, isSet := params[k]
			if !isSet {
				params[k] = v
			}
		}
	}
	iaasName, _ := params["iaas"]
	if iaasName == "" {
		defaultIaaS, err := config.GetString("iaas:default")
		if err != nil {
			defaultIaaS = defaultIaaSProviderName
		}
		iaasName = defaultIaaS
	}
	delete(params, "iaas")
	return CreateMachineForIaaS(iaasName, params)
}

func CreateMachineForIaaS(iaasName string, params map[string]string) (*Machine, error) {
	iaas, err := getIaasProvider(iaasName)
	if err != nil {
		return nil, err
	}
	m, err := iaas.CreateMachine(params)
	if err != nil {
		return nil, err
	}
	m.Iaas = iaasName
	m.CreationParams = params
	err = m.saveToDB()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func ListMachines() ([]Machine, error) {
	coll := collection()
	defer coll.Close()
	var result []Machine
	err := coll.Find(nil).All(&result)
	return result, err
}

func FindMachineByAddress(address string) (Machine, error) {
	coll := collection()
	defer coll.Close()
	var result Machine
	err := coll.Find(bson.M{"address": address}).One(&result)
	return result, err
}

func FindMachineById(id string) (Machine, error) {
	coll := collection()
	defer coll.Close()
	var result Machine
	err := coll.FindId(id).One(&result)
	return result, err
}

func (m *Machine) Destroy() error {
	iaas, err := getIaasProvider(m.Iaas)
	if err != nil {
		return err
	}
	err = iaas.DeleteMachine(m)
	if err != nil {
		return err
	}
	return m.removeFromDB()
}

func (m *Machine) FormatNodeAddress() string {
	protocol, _ := config.GetString("iaas:node-protocol")
	if protocol == "" {
		protocol = "http"
	}
	port, _ := config.GetInt("iaas:node-port")
	if port == 0 {
		port = 2375
	}
	return fmt.Sprintf("%s://%s:%d", protocol, m.Address, port)
}

func (m *Machine) saveToDB() error {
	coll := collection()
	defer coll.Close()
	_, err := coll.UpsertId(m.Id, m)
	return err
}

func (m *Machine) removeFromDB() error {
	coll := collection()
	defer coll.Close()
	return coll.Remove(bson.M{"_id": m.Id})
}

func collection() *storage.Collection {
	name, err := config.GetString("iaas:collection")
	if err != nil {
		name = "iaas_machines"
	}
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
}
