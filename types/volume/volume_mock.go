// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"context"
)

var _ VolumeStorage = (*MockVolumeStorage)(nil)

type MockVolumeStorage struct {
	volumes []Volume
	binds   []VolumeBind

	OnDelete       func(v *Volume) error
	OnSave         func(v *Volume) error
	OnGet          func(name string) (*Volume, error)
	OnListByFilter func(f *Filter) ([]Volume, error)
	OnInsertBind   func(b *VolumeBind) error
	OnRemoveBind   func(id VolumeBindID) error
	OnBinds        func(volumeName string) ([]VolumeBind, error)
	OnBindsForApp  func(volumeName, appName string) ([]VolumeBind, error)
}

func (m *MockVolumeStorage) Save(ctx context.Context, v *Volume) error {
	if m.OnSave == nil {
		for i, storedVolume := range m.volumes {
			if storedVolume.Name == v.Name {
				m.volumes[i] = *v
				return nil
			}
		}
		m.volumes = append(m.volumes, *v)
		return nil
	}

	return m.OnSave(v)
}

func (m *MockVolumeStorage) Delete(ctx context.Context, v *Volume) error {
	if m.OnDelete == nil {
		for i, storedVolume := range m.volumes {
			if storedVolume.Name == v.Name {
				m.volumes = append(m.volumes[:i], m.volumes[i+1:]...)
			}
		}
		return nil
	}

	return m.OnDelete(v)
}

func (m *MockVolumeStorage) Get(ctx context.Context, name string) (*Volume, error) {
	if m.OnGet == nil {
		for _, storedVolume := range m.volumes {
			if storedVolume.Name == name {
				return &storedVolume, nil
			}
		}
		return nil, ErrVolumeNotFound
	}

	return m.OnGet(name)
}

func (m *MockVolumeStorage) ListByFilter(ctx context.Context, f *Filter) ([]Volume, error) {
	if m.OnListByFilter == nil {
		volumes := []Volume{}
		for _, existingVolume := range m.volumes {
			if f == nil ||
				filterMatch(f.Names, existingVolume.Name) ||
				filterMatch(f.Pools, existingVolume.Pool) ||
				filterMatch(f.Teams, existingVolume.TeamOwner) {
				volumes = append(volumes, existingVolume)
			}
		}
		return volumes, nil
	}

	return m.OnListByFilter(f)
}

func (m *MockVolumeStorage) InsertBind(ctx context.Context, b *VolumeBind) error {
	if m.OnInsertBind == nil {
		for _, existingBind := range m.binds {
			if existingBind.ID == b.ID {
				return ErrVolumeBindAlreadyExists
			}
		}
		m.binds = append(m.binds, *b)
		return nil
	}

	return m.OnInsertBind(b)
}

func (m *MockVolumeStorage) RemoveBind(ctx context.Context, id VolumeBindID) error {
	if m.OnRemoveBind == nil {
		for i, existingBind := range m.binds {
			if existingBind.ID == id {
				m.binds = append(m.binds[:i], m.binds[i+1:]...)
				return nil
			}
		}
		return ErrVolumeBindNotFound
	}

	return m.OnRemoveBind(id)
}

func (m *MockVolumeStorage) Binds(ctx context.Context, volumeName string) ([]VolumeBind, error) {
	if m.OnBinds == nil {
		binds := []VolumeBind{}
		for _, bind := range m.binds {
			if bind.ID.Volume == volumeName {
				binds = append(binds, bind)
			}
		}
		return binds, nil
	}

	return m.OnBinds(volumeName)
}

func (m *MockVolumeStorage) BindsForApp(ctx context.Context, volumeName, appName string) ([]VolumeBind, error) {
	if m.OnBindsForApp == nil {
		binds := []VolumeBind{}
		for _, bind := range m.binds {
			if bind.ID.App == appName && volumeName == "" {
				binds = append(binds, bind)
				continue
			}

			if bind.ID.App == appName && bind.ID.Volume == volumeName {
				binds = append(binds, bind)
				continue
			}
		}
		return binds, nil
	}

	return m.OnBindsForApp(volumeName, appName)
}

func (m *MockVolumeStorage) RenameTeam(ctx context.Context, oldTeam, newTeam string) error {
	for i := range m.volumes {
		if m.volumes[i].TeamOwner == oldTeam {
			m.volumes[i].TeamOwner = newTeam
		}
	}
	return nil
}

func filterMatch(values []string, currentValue string) bool {
	for _, v := range values {
		if v == currentValue {
			return true
		}
	}

	return false
}
