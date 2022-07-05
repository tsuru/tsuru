// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	internalConfig "github.com/tsuru/tsuru/config"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	"github.com/tsuru/tsuru/validation"
)

type volumeService struct {
	storage volumeTypes.VolumeStorage
}

var _ volumeTypes.VolumeService = &volumeService{}

func VolumeService() (volumeTypes.VolumeService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}

	return VolumeServiceFromStorage(dbDriver.VolumeStorage), nil
}

func VolumeStorage() (volumeTypes.VolumeStorage, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return dbDriver.VolumeStorage, nil
}

func VolumeServiceFromStorage(vStorage volumeTypes.VolumeStorage) volumeTypes.VolumeService {
	return &volumeService{
		storage: vStorage,
	}
}

func (s *volumeService) Create(ctx context.Context, v *volumeTypes.Volume) error {
	err := s.validateNew(ctx, v)
	if err != nil {
		return err
	}

	err = s.validateProvisioner(ctx, v)
	if err != nil {
		return err
	}

	return s.storage.Save(ctx, v)
}

func (s *volumeService) Update(ctx context.Context, v *volumeTypes.Volume) error {
	err := s.validateProvisioner(ctx, v)
	if err != nil {
		return err
	}
	err = s.validate(ctx, v)
	if err != nil {
		return err
	}

	return s.storage.Save(ctx, v)
}

func (s *volumeService) Get(ctx context.Context, name string) (*volumeTypes.Volume, error) {
	return s.storage.Get(ctx, name)
}

func (s *volumeService) ListByApp(ctx context.Context, appName string) ([]volumeTypes.Volume, error) {
	binds, err := s.storage.BindsForApp(ctx, "", appName) // TODO: test empty volumeName
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(binds) == 0 {
		return []volumeTypes.Volume{}, nil
	}
	var volumeNames []string
	uniqueNames := map[string]bool{}
	for _, bind := range binds {
		if uniqueNames[bind.ID.Volume] {
			continue
		}
		volumeNames = append(volumeNames, bind.ID.Volume)
		uniqueNames[bind.ID.Volume] = true
	}

	return s.storage.ListByFilter(ctx, &volumeTypes.Filter{
		Names: volumeNames,
	})
}

func (s *volumeService) ListByFilter(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
	volumes, err := s.storage.ListByFilter(ctx, f)
	if err != nil {
		return nil, err
	}
	for i := range volumes {
		volumes[i].Binds, err = s.Binds(ctx, &volumes[i])
		if err != nil {
			return nil, err
		}
	}

	return volumes, nil
}

func (s *volumeService) Delete(ctx context.Context, v *volumeTypes.Volume) error {
	binds, err := s.storage.Binds(ctx, v.Name)
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
	return s.storage.Delete(ctx, v)
}

func (s *volumeService) BindApp(ctx context.Context, opts *volumeTypes.BindOpts) error {
	bind := &volumeTypes.VolumeBind{
		ID: volumeTypes.VolumeBindID{
			App:        opts.AppName,
			MountPoint: opts.MountPoint,
			Volume:     opts.Volume.Name,
		},
		ReadOnly: opts.ReadOnly,
	}

	err := s.storage.InsertBind(ctx, bind)
	if err == volumeTypes.ErrVolumeBindAlreadyExists {
		return volumeTypes.ErrVolumeAlreadyBound
	}
	return err
}

func (s *volumeService) UnbindApp(ctx context.Context, opts *volumeTypes.BindOpts) error {
	return s.storage.RemoveBind(ctx, volumeTypes.VolumeBindID{
		App:        opts.AppName,
		Volume:     opts.Volume.Name,
		MountPoint: opts.MountPoint,
	})
}

func (s *volumeService) Binds(ctx context.Context, v *volumeTypes.Volume) ([]volumeTypes.VolumeBind, error) {
	if v.Binds != nil {
		return v.Binds, nil
	}

	binds, err := s.storage.Binds(ctx, v.Name)
	if err != nil {
		return nil, err
	}
	v.Binds = binds
	return binds, nil
}

func (s *volumeService) BindsForApp(ctx context.Context, v *volumeTypes.Volume, appName string) ([]volumeTypes.VolumeBind, error) {
	if v != nil && v.Binds != nil {
		binds := []volumeTypes.VolumeBind{}
		for _, bind := range v.Binds {
			if bind.ID.App == appName {
				binds = append(binds, bind)
			}
		}
		return binds, nil
	}

	var volumeName string
	if v != nil {
		volumeName = v.Name
	}
	binds, err := s.storage.BindsForApp(ctx, volumeName, appName)
	if err != nil {
		return nil, err
	}
	return binds, nil
}

func (s *volumeService) ListPlans(ctx context.Context) (map[string][]volumeTypes.VolumePlan, error) {
	plans := map[string][]volumeTypes.VolumePlan{}
	plansRaw, err := config.Get("volume-plans")
	if err != nil {
		return plans, nil
	}
	plansMap := asMapStringInterface(internalConfig.ConvertEntries(plansRaw))
	for planName, planProvsRaw := range plansMap {
		for prov, provDataRaw := range asMapStringInterface(planProvsRaw) {
			plans[prov] = append(plans[prov], volumeTypes.VolumePlan{
				Name: planName,
				Opts: asMapStringInterface(provDataRaw),
			})
		}
	}
	return plans, nil
}

func (s *volumeService) CheckPoolVolumeConstraints(ctx context.Context, volume volumeTypes.Volume) error {
	pool, err := pool.GetPoolByName(ctx, volume.Pool)
	if err != nil {
		return err
	}

	vPlans, err := pool.GetVolumePlans()
	if err != nil {
		return err
	}

	for _, vplan := range vPlans {
		if volume.Plan.Name == vplan {
			return nil
		}
	}

	return volumeTypes.ErrVolumePlanNotFound
}

func (s *volumeService) validateNew(ctx context.Context, v *volumeTypes.Volume) error {
	if v.Name == "" {
		return errors.New("volume name cannot be empty")
	}
	if !validation.ValidateName(v.Name) {
		msg := "Invalid volume name, volume name should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return errors.WithStack(&tsuruErrors.ValidationError{Message: msg})
	}
	return s.validate(ctx, v)
}

func (s *volumeService) validate(ctx context.Context, v *volumeTypes.Volume) error {
	p, err := pool.GetPoolByName(ctx, v.Pool)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = servicemanager.Team.FindByName(ctx, v.TeamOwner)
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

	if volumeProv, ok := prov.(provision.VolumeProvisioner); ok {
		err = volumeProv.ValidateVolume(ctx, v)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (s *volumeService) validateProvisioner(ctx context.Context, v *volumeTypes.Volume) error {
	isProv, err := isProvisioned(ctx, v)
	if err != nil {
		return err
	}
	if isProv {
		return volumeTypes.ErrVolumeAlreadyProvisioned
	}
	return nil
}

func isProvisioned(ctx context.Context, v *volumeTypes.Volume) (bool, error) {
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

func asMapStringInterface(val interface{}) map[string]interface{} {
	if val == nil {
		return nil
	}
	if mapVal, ok := val.(map[string]interface{}); ok {
		return mapVal
	}
	return nil
}

func volumePlanKey(planName, provisioner string) string {
	return fmt.Sprintf("volume-plans:%s:%s", planName, provisioner)
}

func RenameTeam(ctx context.Context, oldName, newName string) error {
	volumeStorage, err := VolumeStorage()
	if err != nil {
		return err
	}
	return volumeStorage.RenameTeam(ctx, oldName, newName)
}
