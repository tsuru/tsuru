// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
)

var (
	ErrVolumeNotFound           = errors.New("volume not found")
	ErrVolumeBindAlreadyExists  = errors.New("volume bind already exists")
	ErrVolumeAlreadyBound       = errors.New("volume already bound in mountpoint")
	ErrVolumeBindNotFound       = errors.New("volume bind not found")
	ErrVolumeAlreadyProvisioned = errors.New("updating a volume already provisioned is not supported, a new volume must be created and the old one deleted if necessary")
	ErrVolumePlanNotFound       = errors.New("volume-plan not present in pool constraint")
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
	Opts      map[string]string `bson:",omitempty"`
}

func (v *Volume) UnmarshalPlan(result interface{}) error {
	jsonData, err := json.Marshal(v.Plan.Opts)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(json.Unmarshal(jsonData, result))
}

type VolumeWithBinds struct {
	Volume
	Binds []VolumeBind
}

type BindOpts struct {
	Volume     *Volume
	AppName    string
	MountPoint string
	ReadOnly   bool
}

type Filter struct {
	Teams []string
	Pools []string
	Names []string
}

type VolumeService interface {
	Create(ctx context.Context, v *Volume) error
	Update(ctx context.Context, v *Volume) error
	Delete(ctx context.Context, v *Volume) error
	ListByApp(ctx context.Context, appName string) ([]Volume, error)
	ListByFilter(ctx context.Context, f *Filter) ([]VolumeWithBinds, error)
	ListPlans(ctx context.Context) (map[string][]VolumePlan, error)
	CheckPoolVolumeConstraints(ctx context.Context, volume Volume) error
	Get(ctx context.Context, name string) (*Volume, error)

	BindApp(ctx context.Context, opts *BindOpts) error
	UnbindApp(ctx context.Context, opts *BindOpts) error
	BindsForApp(ctx context.Context, v *Volume, appName string) ([]VolumeBind, error)
	Binds(ctx context.Context, v *Volume) ([]VolumeBind, error)
}

type VolumeStorage interface {
	Save(ctx context.Context, v *Volume) error
	Delete(ctx context.Context, v *Volume) error
	Get(ctx context.Context, name string) (*Volume, error)
	ListByFilter(ctx context.Context, f *Filter) ([]Volume, error)

	InsertBind(ctx context.Context, b *VolumeBind) error
	RemoveBind(ctx context.Context, id VolumeBindID) error
	Binds(ctx context.Context, volumeName string) ([]VolumeBind, error)
	BindsForApp(ctx context.Context, volumeName, appName string) ([]VolumeBind, error)

	RenameTeam(ctx context.Context, oldName, newName string) error
}
