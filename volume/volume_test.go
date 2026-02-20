// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"context"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
)

const baseConfig = `
volume-plans:
  p1:
    fake:
       driver: local
       opt:
         type: nfs
`

type fakePlanOpts struct {
	Driver string
	Opt    struct {
		Type string
	}
}

type fakePlan struct {
	Driver string
	Opt    map[string]string
}

type fakeOtherPlan struct {
	Plugin       string
	StorageClass string `json:"storage-class"`
}

type otherProvisioner struct {
	*provisiontest.FakeProvisioner
}

func (p *otherProvisioner) GetName() string {
	return "other"
}

type volumeProvisioner struct {
	*provisiontest.FakeProvisioner
	deleteCallVolume string
	deleteCallPool   string
	isProvisioned    bool
}

func (p *volumeProvisioner) GetName() string {
	return "volumeprov"
}

func (p *volumeProvisioner) DeleteVolume(ctx context.Context, volName, pool string) error {
	p.deleteCallVolume = volName
	p.deleteCallPool = pool
	return nil
}

func (p *volumeProvisioner) IsVolumeProvisioned(ctx context.Context, name, pool string) (bool, error) {
	return p.isProvisioned, nil
}

func setupTest(t *testing.T) {
	t.Helper()
	err := storagev2.ClearAllCollections(nil)
	require.NoError(t, err)
	provisiontest.ProvisionerInstance.Reset()
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "mypool",
		Provisioner: "fake",
	})
	require.NoError(t, err)
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "otherpool",
		Provisioner: "fake",
	})
	require.NoError(t, err)
	teams := []authTypes.Team{{Name: "myteam"}, {Name: "otherteam"}}
	mockTeamService := &authTypes.MockTeamService{
		OnList: func() ([]authTypes.Team, error) {
			return teams, nil
		},
		OnFindByName: func(name string) (*authTypes.Team, error) {
			for _, t := range teams {
				if name == t.Name {
					return &t, nil
				}
			}
			return nil, authTypes.ErrTeamNotFound
		},
	}
	servicemanager.Team = mockTeamService
	setupConfig(baseConfig)
}

func TestVolumeUnmarshalPlan(t *testing.T) {
	setupTest(t)
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	setupConfig(`
volume-plans:
  nfs:
    fake:
       driver: local
       opt:
         type: nfs
    other:
      plugin: nfs
  ebs:
    fake:
       driver: rexray/ebs
    other:
      storage-class: my-ebs-storage-class
`)

	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "mypool2",
		Provisioner: "other",
	})
	require.NoError(t, err)
	v1 := volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = vs.validate(context.TODO(), &v1)
	require.NoError(t, err)
	var resultFake fakePlan
	err = v1.UnmarshalPlan(&resultFake)
	require.NoError(t, err)
	require.EqualValues(t, fakePlan{
		Driver: "local",
		Opt: map[string]string{
			"type": "nfs",
		},
	}, resultFake)
	v1.Plan.Name = "ebs"
	err = vs.validate(context.TODO(), &v1)
	require.NoError(t, err)
	resultFake = fakePlan{}
	err = v1.UnmarshalPlan(&resultFake)
	require.NoError(t, err)
	require.EqualValues(t, fakePlan{
		Driver: "rexray/ebs",
	}, resultFake)
	v1.Plan.Name = "nfs"
	v1.Pool = "mypool2"
	err = vs.validate(context.TODO(), &v1)
	require.NoError(t, err)
	var resultFakeOther fakeOtherPlan
	err = v1.UnmarshalPlan(&resultFakeOther)
	require.NoError(t, err)
	require.EqualValues(t, fakeOtherPlan{
		Plugin: "nfs",
	}, resultFakeOther)
	v1.Plan.Name = "ebs"
	err = vs.validate(context.TODO(), &v1)
	require.NoError(t, err)
	resultFakeOther = fakeOtherPlan{}
	err = v1.UnmarshalPlan(&resultFakeOther)
	require.NoError(t, err)
	require.EqualValues(t, fakeOtherPlan{
		StorageClass: "my-ebs-storage-class",
	}, resultFakeOther)
}

func TestVolumeCreateAndLoad(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	t.Run("volume name cannot be empty", func(t *testing.T) {
		err := volumeService.Create(context.TODO(), &volumeTypes.Volume{})
		require.ErrorContains(t, err, "volume name cannot be empty")
	})
	t.Run("volume cannot be created in a pool that does not exists", func(t *testing.T) {
		err := volumeService.Create(context.TODO(), &volumeTypes.Volume{Name: "v1"})
		require.ErrorContains(t, err, pool.ErrPoolNotFound.Error())
	})
	t.Run("volume cannot be created with a team that does not exists", func(t *testing.T) {
		err := volumeService.Create(context.TODO(), &volumeTypes.Volume{Name: "v1", Pool: "mypool"})
		require.ErrorContains(t, err, authTypes.ErrTeamNotFound.Error())
	})
	t.Run("volume cannot be created with empty plan name", func(t *testing.T) {
		err := volumeService.Create(context.TODO(), &volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam"})
		require.ErrorContains(t, err, "key \"volume-plans::fake\" not found")
	})
	t.Run("volume cannot be created with plan that does not exists", func(t *testing.T) {
		err := volumeService.Create(context.TODO(), &volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "bogus"}})
		require.ErrorContains(t, err, "key \"volume-plans:bogus:fake\" not found")
	})
	t.Run("simple volume creation", func(t *testing.T) {
		vol := &volumeTypes.Volume{
			Name: "v1",
			Plan: volumeTypes.VolumePlan{
				Name: "p1",
			},
			Pool:      "mypool",
			TeamOwner: "myteam",
		}
		err := volumeService.Create(context.TODO(), vol)
		require.NoError(t, err)
		require.EqualValues(t, map[string]any{
			"driver": "local",
			"opt": map[string]any{
				"type": "nfs",
			},
		}, vol.Plan.Opts)

		volumeRegister, err := volumeService.Get(context.TODO(), vol.Name)
		require.NoError(t, err)
		require.EqualValues(t, vol, volumeRegister)
		var planOpts fakePlanOpts
		err = volumeRegister.UnmarshalPlan(&planOpts)
		require.NoError(t, err)
		require.EqualValues(t, fakePlanOpts{
			Driver: "local",
			Opt:    struct{ Type string }{Type: "nfs"},
		}, planOpts)
	})
	t.Run("volume creation with options", func(t *testing.T) {
		vol := &volumeTypes.Volume{
			Name: "v1",
			Plan: volumeTypes.VolumePlan{
				Name: "p1",
			},
			Pool:      "mypool",
			TeamOwner: "myteam",
			Opts:      map[string]string{"opt1": "val1"},
		}
		err := volumeService.Create(context.TODO(), vol)
		require.NoError(t, err)
		require.EqualValues(t, map[string]any{
			"driver": "local",
			"opt": map[string]any{
				"type": "nfs",
			},
		}, vol.Plan.Opts)

		volumeRegister, err := volumeService.Get(context.TODO(), vol.Name)
		require.NoError(t, err)
		require.EqualValues(t, vol, volumeRegister)
		var planOpts fakePlanOpts
		err = volumeRegister.UnmarshalPlan(&planOpts)
		require.NoError(t, err)
		require.EqualValues(t, fakePlanOpts{
			Driver: "local",
			Opt:    struct{ Type string }{Type: "nfs"},
		}, planOpts)
	})
}

func TestVolumeUpdateAndLoad(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	t.Run("volume cannot be updated if pool cannot be found", func(t *testing.T) {
		err := volumeService.Update(context.TODO(), &volumeTypes.Volume{Name: "v1"})
		require.ErrorContains(t, err, pool.ErrPoolNotFound.Error())
	})
	t.Run("volume cannot be updated if team cannot be found", func(t *testing.T) {
		err := volumeService.Update(context.TODO(), &volumeTypes.Volume{Name: "v1", Pool: "mypool"})
		require.ErrorContains(t, err, authTypes.ErrTeamNotFound.Error())
	})
	t.Run("volume cannot be updated if plan is empty", func(t *testing.T) {
		err := volumeService.Update(context.TODO(), &volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam"})
		require.ErrorContains(t, err, "key \"volume-plans::fake\" not found")
	})
	t.Run("volume cannot be updated if plan does not exists", func(t *testing.T) {
		err := volumeService.Update(context.TODO(), &volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "bogus"}})
		require.ErrorContains(t, err, "key \"volume-plans:bogus:fake\" not found")
	})
	t.Run("simple volume update", func(t *testing.T) {
		vol := &volumeTypes.Volume{
			Name: "v1",
			Plan: volumeTypes.VolumePlan{
				Name: "p1",
			},
			Pool:      "mypool",
			TeamOwner: "myteam",
		}
		err := volumeService.Update(context.TODO(), vol)
		require.NoError(t, err)
		require.EqualValues(t, map[string]any{
			"driver": "local",
			"opt": map[string]any{
				"type": "nfs",
			},
		}, vol.Plan.Opts)
		volumeRegister, err := volumeService.Get(context.TODO(), vol.Name)
		require.NoError(t, err)
		require.EqualValues(t, vol, volumeRegister)
		var planOpts fakePlanOpts
		err = volumeRegister.UnmarshalPlan(&planOpts)
		require.NoError(t, err)
		require.EqualValues(t, fakePlanOpts{
			Driver: "local",
			Opt:    struct{ Type string }{Type: "nfs"},
		}, planOpts)
	})
	t.Run("volume update with options", func(t *testing.T) {
		vol := &volumeTypes.Volume{
			Name: "v1",
			Plan: volumeTypes.VolumePlan{
				Name: "p1",
			},
			Pool:      "mypool",
			TeamOwner: "myteam",
			Opts:      map[string]string{"opt1": "val1"},
		}
		err := volumeService.Update(context.TODO(), vol)
		require.NoError(t, err)
		require.EqualValues(t, map[string]any{
			"driver": "local",
			"opt": map[string]any{
				"type": "nfs",
			},
		}, vol.Plan.Opts)
		volumeRegister, err := volumeService.Get(context.TODO(), vol.Name)
		require.NoError(t, err)
		require.EqualValues(t, vol, volumeRegister)
		var planOpts fakePlanOpts
		err = volumeRegister.UnmarshalPlan(&planOpts)
		require.NoError(t, err)
		require.EqualValues(t, fakePlanOpts{
			Driver: "local",
			Opt:    struct{ Type string }{Type: "nfs"},
		}, planOpts)
	})
}

func TestVolumeBindApp(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	vol := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := volumeService.Create(context.TODO(), &vol)
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt1",
		ReadOnly:   true,
	})
	require.NoError(t, err)
	binds, err := volumeService.Binds(context.TODO(), &vol)
	require.NoError(t, err)
	expected := []volumeTypes.VolumeBind{{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: true}}
	require.EqualValues(t, expected, binds)
	volumeRegister, err := volumeService.Get(context.TODO(), vol.Name)
	require.NoError(t, err)
	binds, err = volumeService.Binds(context.TODO(), volumeRegister)
	require.NoError(t, err)
	require.EqualValues(t, expected, binds)
}

func TestVolumeBindAppMultipleMounts(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	vol := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := volumeService.Create(context.TODO(), &vol)
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt1",
		ReadOnly:   false,
	})
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp2",
		MountPoint: "/mnt1",
		ReadOnly:   false,
	})
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	require.ErrorIs(t, err, volumeTypes.ErrVolumeAlreadyBound)
	expected := []volumeTypes.VolumeBind{
		{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "myapp2", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt2", Volume: "v1"}, ReadOnly: true},
	}
	binds, err := volumeService.Binds(context.TODO(), &vol)
	require.NoError(t, err)
	require.EqualValues(t, expected, binds)
}

func TestLoadBindsForApp(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	vol := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := volumeService.Create(context.TODO(), &vol)
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt1",
	})
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp2",
		MountPoint: "/mnt1",
	})
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	require.ErrorIs(t, err, volumeTypes.ErrVolumeAlreadyBound)
	expected := []volumeTypes.VolumeBind{
		{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt2", Volume: "v1"}, ReadOnly: true},
	}
	binds, err := volumeService.BindsForApp(context.TODO(), &vol, "myapp")
	require.NoError(t, err)
	require.EqualValues(t, expected, binds)
}

func TestVolumeUnbindApp(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	vol := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := volumeService.Create(context.TODO(), &vol)
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt1",
		ReadOnly:   true,
	})
	require.NoError(t, err)
	err = volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	require.NoError(t, err)
	err = volumeService.UnbindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt1",
	})
	require.NoError(t, err)
	binds, err := volumeService.Binds(context.TODO(), &vol)
	require.NoError(t, err)
	expected := []volumeTypes.VolumeBind{
		{
			ID: volumeTypes.VolumeBindID{
				App:        "myapp",
				MountPoint: "/mnt2",
				Volume:     "v1",
			},
			ReadOnly: true,
		},
	}
	require.EqualValues(t, expected, binds)
	err = volumeService.UnbindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &vol,
		AppName:    "myapp",
		MountPoint: "/mnt999",
	})
	require.ErrorIs(t, err, volumeTypes.ErrVolumeBindNotFound)
}

func TestListByApp(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	volumes := []volumeTypes.Volume{
		{
			Name:      "v1",
			Plan:      volumeTypes.VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v2",
			Plan:      volumeTypes.VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v3",
			Plan:      volumeTypes.VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
		},
	}
	binds := []volumeTypes.VolumeBind{
		{ID: volumeTypes.VolumeBindID{App: "app1", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "app1", MountPoint: "/mnt2", Volume: "v1"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "app2", MountPoint: "/mnt1", Volume: "v2"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "app1", MountPoint: "/mnt1", Volume: "v3"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "app3", MountPoint: "/mnt1", Volume: "v3"}, ReadOnly: false},
	}
	for i, vol := range volumes {
		err := volumeService.Create(context.TODO(), &vol)
		require.NoError(t, err)

		volumes[i].Plan.Opts = map[string]any{
			"driver": "local",
			"opt": map[string]any{
				"type": "nfs",
			},
		}
		for _, b := range binds {
			if b.ID.Volume == vol.Name {
				err := volumeService.BindApp(context.TODO(), &volumeTypes.BindOpts{
					Volume:     &vol,
					AppName:    b.ID.App,
					MountPoint: b.ID.MountPoint,
					ReadOnly:   b.ReadOnly,
				})
				require.NoError(t, err)
			}
		}
	}
	appVolumes, err := volumeService.ListByApp(context.TODO(), "app1")
	require.NoError(t, err)
	sort.Slice(appVolumes, func(i, j int) bool { return appVolumes[i].Name < appVolumes[j].Name })
	require.EqualValues(t, []volumeTypes.Volume{volumes[0], volumes[2]}, appVolumes)
	appVolumes, err = volumeService.ListByApp(context.TODO(), "app2")
	require.NoError(t, err)
	require.EqualValues(t, []volumeTypes.Volume{volumes[1]}, appVolumes)
	appVolumes, err = volumeService.ListByApp(context.TODO(), "app3")
	require.NoError(t, err)
	require.EqualValues(t, []volumeTypes.Volume{volumes[2]}, appVolumes)
	appVolumes, err = volumeService.ListByApp(context.TODO(), "app4")
	require.NoError(t, err)
	require.Len(t, appVolumes, 0)
}

func TestVolumeDelete(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	vol := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := volumeService.Create(context.TODO(), &vol)
	require.NoError(t, err)
	err = volumeService.Delete(context.TODO(), &vol)
	require.NoError(t, err)
	_, err = volumeService.Get(context.TODO(), vol.Name)
	require.ErrorIs(t, err, volumeTypes.ErrVolumeNotFound)
}

func TestVolumeUpdateAlreadyProvisioned(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	setupConfig(`
volume-plans:
  p1:
    volumeprov:
       driver: local
`)
	volumeProv := volumeProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}
	provision.Register("volumeprov", func() (provision.Provisioner, error) {
		return &volumeProv, nil
	})
	defer provision.Unregister("volumeprov")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "volumepool",
		Provisioner: "volumeprov",
	})
	require.NoError(t, err)
	vol := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "volumepool",
		TeamOwner: "myteam",
	}
	err = volumeService.Create(context.TODO(), &vol)
	require.NoError(t, err)
	volumeProv.isProvisioned = true
	err = volumeService.Create(context.TODO(), &vol)
	require.ErrorIs(t, err, volumeTypes.ErrVolumeAlreadyProvisioned)
}

func TestVolumeDeleteWithVolumeProvisioner(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	setupConfig(`
volume-plans:
  p1:
    volumeprov:
       driver: local
`)
	volumeProv := volumeProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}
	provision.Register("volumeprov", func() (provision.Provisioner, error) {
		return &volumeProv, nil
	})
	defer provision.Unregister("volumeprov")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "volumepool",
		Provisioner: "volumeprov",
	})
	require.NoError(t, err)
	vol := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "volumepool",
		TeamOwner: "myteam",
	}
	err = volumeService.Create(context.TODO(), &vol)
	require.NoError(t, err)
	err = volumeService.Delete(context.TODO(), &vol)
	require.NoError(t, err)
	require.Equal(t, "v1", volumeProv.deleteCallVolume)
	require.Equal(t, "volumepool", volumeProv.deleteCallPool)
	_, err = volumeService.Get(context.TODO(), vol.Name)
	require.ErrorIs(t, err, volumeTypes.ErrVolumeNotFound)
}

func TestListByFilter(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	volumes := []volumeTypes.Volume{
		{
			Name:      "v1",
			Plan:      volumeTypes.VolumePlan{Name: "p1"},
			Binds:     []volumeTypes.VolumeBind{},
			Pool:      "mypool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v2",
			Plan:      volumeTypes.VolumePlan{Name: "p1"},
			Binds:     []volumeTypes.VolumeBind{},
			Pool:      "otherpool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v3",
			Plan:      volumeTypes.VolumePlan{Name: "p1"},
			Binds:     []volumeTypes.VolumeBind{},
			Pool:      "mypool",
			TeamOwner: "otherteam",
		},
	}
	for i, vol := range volumes {
		err := volumeService.Create(context.TODO(), &vol)
		require.NoError(t, err)
		volumes[i].Plan.Opts = map[string]any{
			"driver": "local",
			"opt": map[string]any{
				"type": "nfs",
			},
		}
	}
	tests := []struct {
		filter   *volumeTypes.Filter
		expected []volumeTypes.Volume
	}{
		{filter: nil, expected: volumes},
		{filter: &volumeTypes.Filter{Names: []string{"v1", "v2"}}, expected: volumes[:2]},
		{filter: &volumeTypes.Filter{Names: []string{"v1", "vx"}}, expected: volumes[:1]},
		{filter: &volumeTypes.Filter{Names: []string{"v1", "vx"}, Teams: []string{"myteam"}}, expected: volumes[:2]},
		{filter: &volumeTypes.Filter{Names: []string{"v1", "vx"}, Pools: []string{"otherpool"}}, expected: volumes[:2]},
		{filter: &volumeTypes.Filter{Pools: []string{"otherpool", "mypool"}}, expected: volumes},
	}
	for _, tt := range tests {
		vols, err := volumeService.ListByFilter(context.TODO(), tt.filter)
		require.NoError(t, err)
		sort.Slice(vols, func(i, j int) bool { return vols[i].Name < vols[j].Name })
		require.EqualValues(t, tt.expected, vols)
	}
}

func TestVolumeValidateNew(t *testing.T) {
	setupTest(t)
	vol := volumeTypes.Volume{Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "p1"}}
	nameErr := &tsuruErrors.ValidationError{Message: "Invalid volume name, volume name should have at most 40 " +
		"characters, containing only lower case letters, numbers or dashes, " +
		"starting with a letter."}
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	tt := []struct {
		name        string
		expectedErr error
	}{
		{"volume1", nil},
		{"volume1", nil},
		{"MYVOLUME", nameErr},
		{"volume_1", nameErr},
		{"123volume", nameErr},
		{"an *invalid* name", nameErr},
		{"volume-with-a-name-longer-than-40-characters", nameErr},
		{"volume-with-exactly-40-characters-123456", nil},
	}
	for _, test := range tt {
		vol.Name = test.name
		err := volumeService.Create(context.TODO(), &vol)
		if test.expectedErr == nil {
			require.NoError(t, err, "volumeName: %s", test.name)
			continue
		}
		require.ErrorContains(t, errors.Cause(err), test.expectedErr.Error(), "volumeName: %s", test.name)
	}
}

func TestVolumeValidate(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	setupConfig(`
volume-plans:
  nfs:
    fake:
       driver: local
       opt:
         type: nfs
    other:
      plugin: nfs
  ebs:
    fake:
       driver: rexray/ebs
    other:
      storage-class: my-ebs-storage-class
`)
	tt := []struct {
		volume      volumeTypes.Volume
		expectedErr error
	}{
		{volumeTypes.Volume{Name: "volume1", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "nfs"}}, nil},
		{volumeTypes.Volume{Name: "volume-with-a-name-longer-than-40-characters", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "nfs"}}, nil},
		{volumeTypes.Volume{Name: "volume1", Pool: "invalidpool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "nfs"}}, pool.ErrPoolNotFound},
		{volumeTypes.Volume{Name: "volume1", Pool: "mypool", TeamOwner: "invalidteam", Plan: volumeTypes.VolumePlan{Name: "nfs"}}, authTypes.ErrTeamNotFound},
		{volumeTypes.Volume{Name: "volume1", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "invalidplan"}}, config.ErrKeyNotFound{Key: "volume-plans:invalidplan:fake"}},
	}
	for _, test := range tt {
		err := volumeService.validate(context.TODO(), &test.volume)
		if test.expectedErr == nil {
			require.NoError(t, err, "volumeName: %s", test.volume.Name)
			continue
		}
		require.ErrorContains(t, errors.Cause(err), test.expectedErr.Error(), "volumeName: %s", test.volume.Name)
	}
}

func TestRenameTeam(t *testing.T) {
	setupTest(t)
	volumeService := &volumeService{storage: &volumeTypes.MockVolumeStorage{}}
	vol1 := volumeTypes.Volume{Name: "v1", Plan: volumeTypes.VolumePlan{Name: "p1"}, Pool: "mypool", TeamOwner: "myteam"}
	err := volumeService.Create(context.TODO(), &vol1)
	require.NoError(t, err)
	vol2 := volumeTypes.Volume{Name: "v2", Plan: volumeTypes.VolumePlan{Name: "p1"}, Pool: "mypool", TeamOwner: "otherteam"}
	err = volumeService.Create(context.TODO(), &vol2)
	require.NoError(t, err)
	err = volumeService.storage.RenameTeam(context.TODO(), "myteam", "mynewteam")
	require.NoError(t, err)
	vols, err := volumeService.ListByFilter(context.TODO(), nil)
	require.NoError(t, err)
	sort.Slice(vols, func(i, j int) bool { return vols[i].Name < vols[j].Name })
	require.Equal(t, "mynewteam", vols[0].TeamOwner)
	require.Equal(t, "otherteam", vols[1].TeamOwner)
}
