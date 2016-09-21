// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package install

import (
	"fmt"

	"github.com/tsuru/tsuru/db"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type ErrHostNotFound struct {
	Name string
}

func (e *ErrHostNotFound) Error() string {
	return fmt.Sprintf("host %s not found", e.Name)
}

type ErrHostAlreadyExists struct {
	Name string
}

func (e *ErrHostAlreadyExists) Error() string {
	return fmt.Sprintf("host %s already exists", e.Name)
}

type Host struct {
	Name          string
	Driver        map[string]interface{}
	DriverName    string
	SSHPrivateKey string
	CaCert        string
	CaPrivateKey  string
}

func AddHost(h *Host) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.InstallHosts().Insert(h)
	if mgo.IsDup(err) {
		return &ErrHostAlreadyExists{Name: h.Name}
	}
	return err
}

func GetHostByName(name string) (*Host, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var host *Host
	err = conn.InstallHosts().Find(bson.M{"name": name}).One(&host)
	if err == mgo.ErrNotFound {
		return nil, &ErrHostNotFound{Name: name}
	}
	return host, err
}

func ListHosts() ([]*Host, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var hosts []*Host
	err = conn.InstallHosts().Find(nil).All(&hosts)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}
