// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
)

type templateData struct {
	Name  string
	Value string
}

type Template struct {
	Name     string `bson:"_id"`
	IaaSName string
	Data     []templateData
}

func NewTemplate(name string, iaasName string, data map[string]string) (*Template, error) {
	t := Template{Name: name, IaaSName: iaasName}
	for k, v := range data {
		t.Data = append(t.Data, templateData{Name: k, Value: v})
	}
	return &t, t.saveToDB()
}

func FindTemplate(name string) (*Template, error) {
	coll := template_collection()
	defer coll.Close()
	var template Template
	err := coll.FindId(name).One(&template)
	return &template, err
}

func ListTemplates() ([]Template, error) {
	coll := template_collection()
	defer coll.Close()
	var templates []Template
	err := coll.Find(nil).Sort("_id").All(&templates)
	return templates, err
}

func (t *Template) saveToDB() error {
	coll := template_collection()
	defer coll.Close()
	_, err := coll.UpsertId(t.Name, t)
	return err
}

func (t *Template) paramsMap() map[string]string {
	params := map[string]string{}
	for _, item := range t.Data {
		params[item.Name] = item.Value
	}
	params["iaas"] = t.IaaSName
	return params
}

func template_collection() *storage.Collection {
	name, err := config.GetString("iaas:collection")
	if err != nil {
		name = "iaas_machines"
	}
	name += "_templates"
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
}
