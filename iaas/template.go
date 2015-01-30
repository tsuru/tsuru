// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
)

type TemplateData struct {
	Name  string
	Value string
}

type TemplateDataList []TemplateData

func (l TemplateDataList) Len() int           { return len(l) }
func (l TemplateDataList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l TemplateDataList) Less(i, j int) bool { return l[i].Name < l[j].Name }

type Template struct {
	Name     string `bson:"_id"`
	IaaSName string
	Data     TemplateDataList
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

func DestroyTemplate(name string) error {
	coll := template_collection()
	defer coll.Close()
	return coll.RemoveId(name)
}

func (t *Template) Save() error {
	if t.Name == "" {
		return errors.New("template name cannot be empty")
	}
	_, err := GetIaasProvider(t.IaaSName)
	if err != nil {
		return err
	}
	return t.saveToDB()
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
