// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package iaas provides interfaces that need to be satisfied in order to
// implement a new iaas on tsuru.
package iaas

import (
	"fmt"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
)

var (
	iaasDurationBuckets = append([]float64{10, 30}, prometheus.LinearBuckets(60, 60, 10)...)

	machineCreateDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tsuru_iaas_create_duration_seconds",
		Help:    "The machine creation latency distributions.",
		Buckets: iaasDurationBuckets,
	}, []string{"iaas"})

	machineDestroyDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tsuru_iaas_destroy_duration_seconds",
		Help:    "The machine destroy latency distributions.",
		Buckets: iaasDurationBuckets,
	}, []string{"iaas"})

	machineCreateErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_iaas_create_errors_total",
		Help: "The total number of machine creation errors.",
	}, []string{"iaas"})

	machineDestroyErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_iaas_destroy_errors_total",
		Help: "The total number of machine destroy errors.",
	}, []string{"iaas"})

	ErrMachineNotFound = errors.New("machine not found")
)

func init() {
	prometheus.MustRegister(machineCreateDuration)
	prometheus.MustRegister(machineDestroyDuration)
	prometheus.MustRegister(machineCreateErrors)
	prometheus.MustRegister(machineDestroyErrors)
}

type Machine struct {
	Id             string `bson:"_id"`
	Iaas           string
	Status         string
	Address        string
	Port           int
	Protocol       string
	CreationParams map[string]string
	CustomData     map[string]interface{} `json:"-"`
	CaCert         []byte                 `json:"-"`
	ClientCert     []byte                 `json:"-"`
	ClientKey      []byte                 `json:"-"`
}

type DestroyParams struct {
	Force bool
}

func CreateMachine(params map[string]string) (*Machine, error) {
	return CreateMachineForIaaS("", params)
}

func CreateMachineForIaaS(iaasName string, params map[string]string) (*Machine, error) {
	if iaasName == "" {
		iaasName = params["iaas"]
	}
	if iaasName == "" {
		defaultIaaS, err := getDefaultIaasName()
		if err != nil {
			return nil, err
		}
		iaasName = defaultIaaS
	}
	params["iaas"] = iaasName
	iaas, err := getIaasProvider(iaasName)
	if err != nil {
		return nil, err
	}
	t0 := time.Now()
	m, err := iaas.CreateMachine(params)
	machineCreateDuration.WithLabelValues(iaasName).Observe(time.Since(t0).Seconds())
	if err != nil {
		machineCreateErrors.WithLabelValues(iaasName).Inc()
		return nil, err
	}
	params[provision.IaaSIDMetadataName] = m.Id
	m.Iaas = iaasName
	m.CreationParams = params
	err = m.saveToDB(true)
	if err != nil {
		m.Destroy(DestroyParams{})
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
	if err == mgo.ErrNotFound {
		err = ErrMachineNotFound
	}
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
	if err == mgo.ErrNotFound {
		err = ErrMachineNotFound
	}
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
	if err == mgo.ErrNotFound {
		err = ErrMachineNotFound
	}
	return result, err
}

func (m *Machine) Destroy(params DestroyParams) error {
	iaas, err := getIaasProvider(m.Iaas)
	if err != nil {
		return err
	}
	t0 := time.Now()
	err = iaas.DeleteMachine(m)
	machineDestroyDuration.WithLabelValues(m.Iaas).Observe(time.Since(t0).Seconds())
	if err != nil {
		machineDestroyErrors.WithLabelValues(m.Iaas).Inc()
		err = errors.Wrapf(err, "failed to destroy machine in the IaaS")
		if !params.Force {
			return err
		}
		log.Errorf("ignored error due to force: %v", err)
	}
	return m.removeFromDB()
}

func (m *Machine) FormatNodeAddress() string {
	protocol := m.Protocol
	if protocol == "" {
		protocol, _ = config.GetString("iaas:node-protocol")
	}
	if protocol == "" {
		protocol = "http"
	}
	port := m.Port
	if port == 0 {
		port, _ = config.GetInt("iaas:node-port")
	}
	if port == 0 {
		port = 2375
	}
	return fmt.Sprintf("%s://%s:%d", protocol, m.Address, port)
}

func (m *Machine) saveToDB(forceOverwrite bool) error {
	coll, err := collectionEnsureIdx()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(m.Id, m)
	if forceOverwrite && err != nil && mgo.IsDup(err) {
		coll.Remove(bson.M{"address": m.Address, "iaas": m.Iaas})
		_, err = coll.UpsertId(m.Id, m)
	}
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
	return conn.Collection(name), nil
}

func collectionEnsureIdx() (*storage.Collection, error) {
	coll, err := collection()
	if err != nil {
		return nil, err
	}
	index := mgo.Index{
		Key:    []string{"address"},
		Unique: true,
	}
	err = coll.EnsureIndex(index)
	if err != nil {
		coll.Close()
		return nil, errors.Errorf(`Could not create index on address for machines collection.
This can be caused by multiple machines with the same address, please run
"tsuru machine-list" to check for duplicated entries and "tsuru
machine-destroy" to remove them.
original error: %s`, err)
	}
	return coll, nil
}
