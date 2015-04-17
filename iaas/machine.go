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
	"gopkg.in/mgo.v2"
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
	return CreateMachineForIaaS("", params)
}

func CreateMachineForIaaS(iaasName string, params map[string]string) (*Machine, error) {
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
	if iaasName == "" {
		iaasName, _ = params["iaas"]
	}
	if iaasName == "" {
		defaultIaaS, err := config.GetString("iaas:default")
		if err != nil {
			defaultIaaS = defaultIaaSProviderName
		}
		iaasName = defaultIaaS
	}
	params["iaas"] = iaasName
	iaas, err := getIaasProvider(iaasName)
	if err != nil {
		return nil, err
	}
	m, err := iaas.CreateMachine(params)
	if err != nil {
		return nil, err
	}
	params["iaas-id"] = m.Id
	m.Iaas = iaasName
	m.CreationParams = params
	err = m.saveToDB()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func ListMachines() ([]Machine, error) {
	coll, err := collection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var result []Machine
	err = coll.Find(nil).All(&result)
	return result, err
}

// Uses id or address, this is only used because previously we didn't have
// iaas-id in node metadata.
func FindMachineByIdOrAddress(id string, address string) (Machine, error) {
	coll, err := collection()
	if err != nil {
		return Machine{}, err
	}
	defer coll.Close()
	var result Machine
	query := bson.M{}
	if id != "" {
		query["_id"] = id
	} else {
		query["address"] = address
	}
	err = coll.Find(query).One(&result)
	return result, err
}

func FindMachineByAddress(address string) (Machine, error) {
	coll, err := collection()
	if err != nil {
		return Machine{}, err
	}
	defer coll.Close()
	var result Machine
	err = coll.Find(bson.M{"address": address}).One(&result)
	return result, err
}

func FindMachineById(id string) (Machine, error) {
	coll, err := collection()
	if err != nil {
		return Machine{}, err
	}
	defer coll.Close()
	var result Machine
	err = coll.FindId(id).One(&result)
	return result, err
}

func (m *Machine) Destroy() error {
	iaas, err := getIaasProvider(m.Iaas)
	if err != nil {
		return err
	}
	err = iaas.DeleteMachine(m)
	if err != nil {
		log.Errorf("failed to destroy machine in the IaaS: %s", err)
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
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(m.Id, m)
	return err
}

func (m *Machine) removeFromDB() error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Remove(bson.M{"_id": m.Id})
}

func collection() (*storage.Collection, error) {
	name, err := config.GetString("iaas:collection")
	if err != nil {
		name = "iaas_machines"
	}
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
		return nil, err
	}
	coll := conn.Collection(name)
	index := mgo.Index{
		Key:    []string{"address"},
		Unique: true,
	}
	err = coll.EnsureIndex(index)
	if err != nil {
		return nil, fmt.Errorf(`could not create index on address for collection %q. `+
			`this can be caused by multiple entries with the same address, please run "tsuru machine-list" and check for duplicated entries. `+
			`original error: %s`, name, err)
	}
	return coll, nil
}
