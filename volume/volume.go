// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	internalConfig "github.com/tsuru/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrVolumeNotFound = errors.New("volume not found")
)

type VolumePlan struct {
	Name string
	Opts map[string]interface{}
}

type Volume struct {
	Name      string `bson:"_id"`
	Pool      string
	Plan      VolumePlan
	TeamOwner string
	Status    string
	Opts      map[string]string `bson:",omitempty"`
	Apps      []string          `bson:",omitempty"`
}

func (v *Volume) UnmarshalPlan(result interface{}) error {
	jsonData, err := json.Marshal(v.Plan.Opts)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(json.Unmarshal(jsonData, result))
}

func (v *Volume) Validate() error {
	if v.Name == "" {
		return errors.New("volume name cannot be empty")
	}
	pool, err := provision.GetPoolByName(v.Pool)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = auth.GetTeam(v.TeamOwner)
	if err != nil {
		return errors.WithStack(err)
	}
	prov, err := pool.GetProvisioner()
	if err != nil {
		return errors.WithStack(err)
	}
	data, err := config.Get(volumePlanKey(v.Plan.Name, prov.GetName()))
	if err != nil {
		return errors.WithStack(err)
	}
	planOpts, ok := internalConfig.ConvertEntries(data).(map[string]interface{})
	if !ok {
		return errors.Errorf("invalid type for plan opts %T", planOpts)
	}
	v.Plan.Opts = planOpts
	return nil
}

func (v *Volume) Save() error {
	err := v.Validate()
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return errors.WithStack(err)
	}
	defer conn.Close()
	_, err = conn.Volumes().UpsertId(v.Name, v)
	return errors.WithStack(err)
}

func (v *Volume) BindApp(appName string) error {
	conn, err := db.Conn()
	if err != nil {
		return errors.WithStack(err)
	}
	defer conn.Close()
	err = conn.Volumes().UpdateId(v.Name, bson.M{"$addToSet": bson.M{"apps": appName}})
	if err != nil {
		return errors.WithStack(err)
	}
	v.Apps = append(v.Apps, appName)
	return nil
}

func ListByApp(appName string) ([]Volume, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close()
	var volumes []Volume
	err = conn.Volumes().Find(bson.M{"apps": appName}).All(&volumes)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return volumes, nil
}

func Load(name string) (*Volume, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close()
	var v Volume
	err = conn.Volumes().FindId(name).One(&v)
	if err == mgo.ErrNotFound {
		return nil, ErrVolumeNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &v, nil
}

func volumePlanKey(planName, provisioner string) string {
	return fmt.Sprintf("volume-plans:%s:%s", planName, provisioner)
}
