// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	internalConfig "github.com/tsuru/tsuru/config"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/validation"
)

var (
	ErrVolumeNotFound           = errors.New("volume not found")
	ErrVolumeAlreadyBound       = errors.New("volume already bound in mountpoint")
	ErrVolumeBindNotFound       = errors.New("volume bind not found")
	ErrVolumeAlreadyProvisioned = errors.New("updating a volume already provisioned is not supported, a new volume must be created and the old one deleted if necessary")
)

type VolumePlan struct {
	Name string
	Opts map[string]interface{}
}

type VolumeBindID struct {
	App        string
	MountPoint string
	Volume     string
}

type VolumeBind struct {
	ID       VolumeBindID `bson:"_id"`
	ReadOnly bool
}

type Volume struct {
	Name      string `bson:"_id"`
	Pool      string
	Plan      VolumePlan
	TeamOwner string
	Status    string
	Binds     []VolumeBind      `bson:"-"`
	Opts      map[string]string `bson:",omitempty"`

	ctx context.Context
}

func (v *Volume) UnmarshalPlan(result interface{}) error {
	jsonData, err := json.Marshal(v.Plan.Opts)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(json.Unmarshal(jsonData, result))
}

func (v *Volume) validateNew(ctx context.Context) error {
	if v.Name == "" {
		return errors.New("volume name cannot be empty")
	}
	if !validation.ValidateName(v.Name) {
		msg := "Invalid volume name, volume name should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return errors.WithStack(&tsuruErrors.ValidationError{Message: msg})
	}
	return v.validate(ctx)
}

func (v *Volume) validate(ctx context.Context) error {
	p, err := pool.GetPoolByName(ctx, v.Pool)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = servicemanager.Team.FindByName(v.TeamOwner)
	if err != nil {
		return errors.WithStack(err)
	}
	prov, err := p.GetProvisioner()
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

func (v *Volume) Create(ctx context.Context) error {
	err := v.validateNew(ctx)
	if err != nil {
		return err
	}
	return v.save(ctx)
}

func (v *Volume) Update(ctx context.Context) error {
	err := v.validate(ctx)
	if err != nil {
		return err
	}
	return v.save(ctx)
}

func (v *Volume) save(ctx context.Context) error {
	isProv, err := v.isProvisioned(ctx)
	if err != nil {
		return err
	}
	if isProv {
		return ErrVolumeAlreadyProvisioned
	}
	conn, err := db.Conn()
	if err != nil {
		return errors.WithStack(err)
	}
	defer conn.Close()
	_, err = conn.Volumes().UpsertId(v.Name, v)
	return errors.WithStack(err)
}

func (v *Volume) isProvisioned(ctx context.Context) (bool, error) {
	p, err := pool.GetPoolByName(ctx, v.Pool)
	if err != nil {
		return false, errors.WithStack(err)
	}
	prov, err := p.GetProvisioner()
	if err != nil {
		return false, errors.WithStack(err)
	}
	volProv, ok := prov.(provision.VolumeProvisioner)
	if !ok {
		return false, errors.New("provisioner is not a volume provisioner")
	}
	isProv, err := volProv.IsVolumeProvisioned(ctx, v.Name, v.Pool)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return isProv, nil
}

func (v *Volume) BindApp(appName, mountPoint string, readOnly bool) error {
	conn, err := db.Conn()
	if err != nil {
		return errors.WithStack(err)
	}
	defer conn.Close()
	bind := VolumeBind{
		ID: VolumeBindID{
			App:        appName,
			MountPoint: mountPoint,
			Volume:     v.Name,
		},
		ReadOnly: readOnly,
	}
	err = conn.VolumeBinds().Insert(bind)
	if err != nil && mgo.IsDup(err) {
		return ErrVolumeAlreadyBound
	}
	return errors.WithStack(err)
}

func (v *Volume) UnbindApp(appName, mountPoint string) error {
	conn, err := db.Conn()
	if err != nil {
		return errors.WithStack(err)
	}
	defer conn.Close()
	err = conn.VolumeBinds().RemoveId(VolumeBindID{
		App:        appName,
		Volume:     v.Name,
		MountPoint: mountPoint,
	})
	if err == mgo.ErrNotFound {
		return ErrVolumeBindNotFound
	}
	return errors.WithStack(err)
}

func (v *Volume) LoadBindsForApp(appName string) ([]VolumeBind, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close()
	var binds []VolumeBind
	err = conn.VolumeBinds().Find(bson.M{"_id.volume": v.Name, "_id.app": appName}).All(&binds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return binds, nil
}

func (v *Volume) LoadBinds() ([]VolumeBind, error) {
	if v.Binds != nil {
		return v.Binds, nil
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close()
	var binds []VolumeBind
	err = conn.VolumeBinds().Find(bson.M{"_id.volume": v.Name}).All(&binds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	v.Binds = binds
	return binds, nil
}

func (v *Volume) Delete(ctx context.Context) error {
	binds, err := v.LoadBinds()
	if err != nil {
		return err
	}
	if len(binds) > 0 {
		return errors.New("cannot delete volume with existing binds")
	}
	p, err := pool.GetPoolByName(ctx, v.Pool)
	if err != nil {
		return errors.WithStack(err)
	}
	prov, err := p.GetProvisioner()
	if err != nil {
		return errors.WithStack(err)
	}
	if volProv, ok := prov.(provision.VolumeProvisioner); ok {
		err = volProv.DeleteVolume(ctx, v.Name, v.Pool)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	conn, err := db.Conn()
	if err != nil {
		return errors.WithStack(err)
	}
	defer conn.Close()
	return conn.Volumes().RemoveId(v.Name)
}

func ListByApp(appName string) ([]Volume, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close()
	var volumeNames []string
	err = conn.VolumeBinds().Find(bson.M{"_id.app": appName}).Distinct("_id.volume", &volumeNames)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var volumes []Volume
	err = conn.Volumes().Find(bson.M{"_id": bson.M{"$in": volumeNames}}).All(&volumes)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return volumes, nil
}

type Filter struct {
	Teams []string
	Pools []string
	Names []string
}

func ListByFilter(f *Filter) ([]Volume, error) {
	query := bson.M{}
	if f != nil {
		query["$or"] = []bson.M{
			{"_id": bson.M{"$in": f.Names}},
			{"pool": bson.M{"$in": f.Pools}},
			{"teamowner": bson.M{"$in": f.Teams}},
		}
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close()
	var volumes []Volume
	err = conn.Volumes().Find(query).All(&volumes)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for i := range volumes {
		_, err = volumes[i].LoadBinds()
		if err != nil {
			return nil, err
		}
	}
	return volumes, nil
}

func ListPlans() (map[string][]VolumePlan, error) {
	plans := map[string][]VolumePlan{}
	plansRaw, err := config.Get("volume-plans")
	if err != nil {
		return plans, nil
	}
	plansMap := asMapStringInterface(internalConfig.ConvertEntries(plansRaw))
	for planName, planProvsRaw := range plansMap {
		for prov, provDataRaw := range asMapStringInterface(planProvsRaw) {
			plans[prov] = append(plans[prov], VolumePlan{
				Name: planName,
				Opts: asMapStringInterface(provDataRaw),
			})
		}
	}
	return plans, nil
}

func asMapStringInterface(val interface{}) map[string]interface{} {
	if val == nil {
		return nil
	}
	if mapVal, ok := val.(map[string]interface{}); ok {
		return mapVal
	}
	return nil
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

func RenameTeam(ctx context.Context, oldName, newName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Volumes().UpdateAll(bson.M{"teamowner": oldName}, bson.M{"$set": bson.M{"teamowner": newName}})
	return err
}
