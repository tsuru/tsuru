// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"context"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	check "gopkg.in/check.v1"
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

type S struct {
	mockTeamService *authTypes.MockTeamService
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func updateConfig(data string) {
	config.ReadConfigBytes([]byte(data))
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_volume_test")
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

func (s *S) SetUpSuite(c *check.C) {
	otherProv := otherProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}
	provision.Register("other", func() (provision.Provisioner, error) {
		return &otherProv, nil
	})
	updateConfig("")
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	provisiontest.ProvisionerInstance.Reset()
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "mypool",
		Provisioner: "fake",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "otherpool",
		Provisioner: "fake",
	})
	c.Assert(err, check.IsNil)
	teams := []authTypes.Team{{Name: "myteam"}, {Name: "otherteam"}}
	s.mockTeamService = &authTypes.MockTeamService{
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
	servicemanager.Team = s.mockTeamService
	updateConfig(baseConfig)
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.DefaultDatabase())
	c.Assert(err, check.IsNil)
}

func (s *S) TestVolumeUnmarshalPlan(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	updateConfig(`
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
	type fakePlan struct {
		Driver string
		Opt    map[string]string
	}
	type fakeOtherPlan struct {
		Plugin       string
		StorageClass string `json:"storage-class"`
	}
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "mypool2",
		Provisioner: "other",
	})
	c.Assert(err, check.IsNil)
	v1 := volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = vs.validate(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	var resultFake fakePlan
	err = v1.UnmarshalPlan(&resultFake)
	c.Assert(err, check.IsNil)
	c.Assert(resultFake, check.DeepEquals, fakePlan{
		Driver: "local",
		Opt: map[string]string{
			"type": "nfs",
		},
	})
	v1.Plan.Name = "ebs"
	err = vs.validate(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	resultFake = fakePlan{}
	err = v1.UnmarshalPlan(&resultFake)
	c.Assert(err, check.IsNil)
	c.Assert(resultFake, check.DeepEquals, fakePlan{
		Driver: "rexray/ebs",
	})
	v1.Plan.Name = "nfs"
	v1.Pool = "mypool2"
	err = vs.validate(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	var resultFakeOther fakeOtherPlan
	err = v1.UnmarshalPlan(&resultFakeOther)
	c.Assert(err, check.IsNil)
	c.Assert(resultFakeOther, check.DeepEquals, fakeOtherPlan{
		Plugin: "nfs",
	})
	v1.Plan.Name = "ebs"
	err = vs.validate(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	resultFakeOther = fakeOtherPlan{}
	err = v1.UnmarshalPlan(&resultFakeOther)
	c.Assert(err, check.IsNil)
	c.Assert(resultFakeOther, check.DeepEquals, fakeOtherPlan{
		StorageClass: "my-ebs-storage-class",
	})
}

func (s *S) TestVolumeCreateLoad(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}

	tests := []struct {
		v   volumeTypes.Volume
		err string
	}{
		{
			v:   volumeTypes.Volume{},
			err: "volume name cannot be empty",
		},
		{
			v:   volumeTypes.Volume{Name: "v1"},
			err: pool.ErrPoolNotFound.Error(),
		},
		{
			v:   volumeTypes.Volume{Name: "v1", Pool: "mypool"},
			err: authTypes.ErrTeamNotFound.Error(),
		},
		{
			v:   volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam"},
			err: "key \"volume-plans::fake\" not found",
		},
		{
			v:   volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "bogus"}},
			err: "key \"volume-plans:bogus:fake\" not found",
		},
		{
			v: volumeTypes.Volume{
				Name: "v1",
				Plan: volumeTypes.VolumePlan{
					Name: "p1",
				},
				Pool:      "mypool",
				TeamOwner: "myteam",
			},
		},
		{
			v: volumeTypes.Volume{
				Name: "v1",
				Plan: volumeTypes.VolumePlan{
					Name: "p1",
				},
				Pool:      "mypool",
				TeamOwner: "myteam",
				Opts:      map[string]string{"opt1": "val1"},
			},
		},
	}
	for i, tt := range tests {
		err := vs.Create(context.TODO(), &tt.v)
		if tt.err != "" {
			c.Assert(err, check.ErrorMatches, tt.err)
			continue
		}
		c.Assert(err, check.IsNil)
		c.Assert(tt.v.Plan.Opts, check.DeepEquals, map[string]interface{}{
			"driver": "local",
			"opt": map[string]interface{}{
				"type": "nfs",
			},
		})
		dbV, err := vs.Get(context.TODO(), tt.v.Name)
		c.Assert(err, check.IsNil, check.Commentf("test %d", i))
		c.Assert(dbV, check.DeepEquals, &tt.v)
		var planOpts fakePlanOpts
		err = dbV.UnmarshalPlan(&planOpts)
		c.Assert(err, check.IsNil)
		c.Assert(planOpts, check.DeepEquals, fakePlanOpts{
			Driver: "local",
			Opt:    struct{ Type string }{Type: "nfs"},
		})
	}
}

func (s *S) TestVolumeUpdateLoad(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}

	tests := []struct {
		v   volumeTypes.Volume
		err string
	}{
		{
			v:   volumeTypes.Volume{Name: "v1"},
			err: pool.ErrPoolNotFound.Error(),
		},
		{
			v:   volumeTypes.Volume{Name: "v1", Pool: "mypool"},
			err: authTypes.ErrTeamNotFound.Error(),
		},
		{
			v:   volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam"},
			err: "key \"volume-plans::fake\" not found",
		},
		{
			v:   volumeTypes.Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "bogus"}},
			err: "key \"volume-plans:bogus:fake\" not found",
		},
		{
			v: volumeTypes.Volume{
				Name: "v1",
				Plan: volumeTypes.VolumePlan{
					Name: "p1",
				},
				Pool:      "mypool",
				TeamOwner: "myteam",
			},
		},
		{
			v: volumeTypes.Volume{
				Name: "v1",
				Plan: volumeTypes.VolumePlan{
					Name: "p1",
				},
				Pool:      "mypool",
				TeamOwner: "myteam",
				Opts:      map[string]string{"opt1": "val1"},
			},
		},
	}
	for i, tt := range tests {
		err := vs.Update(context.TODO(), &tt.v)
		if tt.err != "" {
			c.Assert(err, check.ErrorMatches, tt.err)
			continue
		}
		c.Assert(err, check.IsNil)
		c.Assert(tt.v.Plan.Opts, check.DeepEquals, map[string]interface{}{
			"driver": "local",
			"opt": map[string]interface{}{
				"type": "nfs",
			},
		})
		dbV, err := vs.Get(context.TODO(), tt.v.Name)
		c.Assert(err, check.IsNil, check.Commentf("test %d", i))
		c.Assert(dbV, check.DeepEquals, &tt.v)
		var planOpts fakePlanOpts
		err = dbV.UnmarshalPlan(&planOpts)
		c.Assert(err, check.IsNil)
		c.Assert(planOpts, check.DeepEquals, fakePlanOpts{
			Driver: "local",
			Opt:    struct{ Type string }{Type: "nfs"},
		})
	}
}

func (s *S) TestVolumeBindApp(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	v := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := vs.Create(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt1",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)
	binds, err := vs.Binds(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	expected := []volumeTypes.VolumeBind{{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: true}}
	c.Assert(binds, check.DeepEquals, expected)
	dbV, err := vs.Get(context.TODO(), v.Name)
	c.Assert(err, check.IsNil)
	binds, err = vs.Binds(context.TODO(), dbV)
	c.Assert(err, check.IsNil)
	c.Assert(binds, check.DeepEquals, expected)
}

func (s *S) TestVolumeBindAppMultipleMounts(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	v := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := vs.Create(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt1",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp2",
		MountPoint: "/mnt1",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	c.Assert(err, check.Equals, volumeTypes.ErrVolumeAlreadyBound)
	expected := []volumeTypes.VolumeBind{
		{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "myapp2", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt2", Volume: "v1"}, ReadOnly: true},
	}
	binds, err := vs.Binds(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	c.Assert(binds, check.DeepEquals, expected)
}

func (s *S) TestLoadBindsForApp(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	v := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := vs.Create(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt1",
	})
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp2",
		MountPoint: "/mnt1",
	})
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	c.Assert(err, check.Equals, volumeTypes.ErrVolumeAlreadyBound)
	expected := []volumeTypes.VolumeBind{
		{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: volumeTypes.VolumeBindID{App: "myapp", MountPoint: "/mnt2", Volume: "v1"}, ReadOnly: true},
	}
	binds, err := vs.BindsForApp(context.TODO(), &v, "myapp")
	c.Assert(err, check.IsNil)
	c.Assert(binds, check.DeepEquals, expected)
}

func (s *S) TestVolumeUnbindApp(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	v := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := vs.Create(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt1",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)
	err = vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt2",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)
	err = vs.UnbindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt1",
	})
	c.Assert(err, check.IsNil)
	binds, err := vs.Binds(context.TODO(), &v)
	c.Assert(err, check.IsNil)
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
	c.Assert(binds, check.DeepEquals, expected)
	err = vs.UnbindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    "myapp",
		MountPoint: "/mnt999",
	})
	c.Assert(err, check.Equals, volumeTypes.ErrVolumeBindNotFound)
}

func (s *S) TestListByApp(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
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
	for i, v := range volumes {
		err := vs.Create(context.TODO(), &v)
		c.Assert(err, check.IsNil)

		volumes[i].Plan.Opts = map[string]interface{}{
			"driver": "local",
			"opt": map[string]interface{}{
				"type": "nfs",
			},
		}
		for _, b := range binds {
			if b.ID.Volume == v.Name {
				err := vs.BindApp(context.TODO(), &volumeTypes.BindOpts{
					Volume:     &v,
					AppName:    b.ID.App,
					MountPoint: b.ID.MountPoint,
					ReadOnly:   b.ReadOnly,
				})
				c.Assert(err, check.IsNil)
			}
		}
	}
	appVolumes, err := vs.ListByApp(context.TODO(), "app1")
	c.Assert(err, check.IsNil)
	sort.Slice(appVolumes, func(i, j int) bool { return appVolumes[i].Name < appVolumes[j].Name })
	c.Assert(appVolumes, check.DeepEquals, []volumeTypes.Volume{volumes[0], volumes[2]})
	appVolumes, err = vs.ListByApp(context.TODO(), "app2")
	c.Assert(err, check.IsNil)
	c.Assert(appVolumes, check.DeepEquals, []volumeTypes.Volume{volumes[1]})
	appVolumes, err = vs.ListByApp(context.TODO(), "app3")
	c.Assert(err, check.IsNil)
	c.Assert(appVolumes, check.DeepEquals, []volumeTypes.Volume{volumes[2]})
	appVolumes, err = vs.ListByApp(context.TODO(), "app4")
	c.Assert(err, check.IsNil)
	c.Assert(appVolumes, check.HasLen, 0)
}

func (s *S) TestVolumeDelete(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	v := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := vs.Create(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	err = vs.Delete(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	_, err = vs.Get(context.TODO(), v.Name)
	c.Assert(err, check.Equals, volumeTypes.ErrVolumeNotFound)
}

func (s *S) TestVolumeUpdateAlreadyProvisioned(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	updateConfig(`
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
	c.Assert(err, check.IsNil)
	v := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "volumepool",
		TeamOwner: "myteam",
	}
	err = vs.Create(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	volumeProv.isProvisioned = true
	err = vs.Create(context.TODO(), &v)
	c.Assert(err, check.Equals, volumeTypes.ErrVolumeAlreadyProvisioned)
}

func (s *S) TestVolumeDeleteWithVolumeProvisioner(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	updateConfig(`
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
	c.Assert(err, check.IsNil)
	v := volumeTypes.Volume{
		Name:      "v1",
		Plan:      volumeTypes.VolumePlan{Name: "p1"},
		Pool:      "volumepool",
		TeamOwner: "myteam",
	}
	err = vs.Create(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	err = vs.Delete(context.TODO(), &v)
	c.Assert(err, check.IsNil)
	c.Assert(volumeProv.deleteCallVolume, check.Equals, "v1")
	c.Assert(volumeProv.deleteCallPool, check.Equals, "volumepool")
	_, err = vs.Get(context.TODO(), v.Name)
	c.Assert(err, check.Equals, volumeTypes.ErrVolumeNotFound)
}

func (s *S) TestListByFilter(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
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
			Pool:      "otherpool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v3",
			Plan:      volumeTypes.VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "otherteam",
		},
	}
	for i, v := range volumes {
		err := vs.Create(context.TODO(), &v)
		c.Assert(err, check.IsNil)
		volumes[i].Plan.Opts = map[string]interface{}{
			"driver": "local",
			"opt": map[string]interface{}{
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
		vols, err := vs.ListByFilter(context.TODO(), tt.filter)
		c.Assert(err, check.IsNil)
		sort.Slice(vols, func(i, j int) bool { return vols[i].Name < vols[j].Name })
		c.Assert(vols, check.DeepEquals, tt.expected)
	}
}

func (s *S) TestVolumeValidateNew(c *check.C) {
	vol := volumeTypes.Volume{Pool: "mypool", TeamOwner: "myteam", Plan: volumeTypes.VolumePlan{Name: "p1"}}
	msg := "Invalid volume name, volume name should have at most 40 " +
		"characters, containing only lower case letters, numbers or dashes, " +
		"starting with a letter."
	nameErr := &tsuruErrors.ValidationError{Message: msg}
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
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
	for _, t := range tt {
		vol.Name = t.name
		err := vs.Create(context.TODO(), &vol)
		c.Check(errors.Cause(err), check.DeepEquals, t.expectedErr, check.Commentf(t.name))
	}
}

func (s *S) TestVolumeValidate(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	updateConfig(`
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
	for _, t := range tt {
		c.Check(errors.Cause(vs.validate(context.TODO(), &t.volume)), check.DeepEquals, t.expectedErr, check.Commentf(t.volume.Name))
	}
}

func (s *S) TestRenameTeam(c *check.C) {
	vs := &volumeService{
		storage: &volumeTypes.MockVolumeStorage{},
	}
	v1 := volumeTypes.Volume{Name: "v1", Plan: volumeTypes.VolumePlan{Name: "p1"}, Pool: "mypool", TeamOwner: "myteam"}
	err := vs.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	v2 := volumeTypes.Volume{Name: "v2", Plan: volumeTypes.VolumePlan{Name: "p1"}, Pool: "mypool", TeamOwner: "otherteam"}
	err = vs.Create(context.TODO(), &v2)
	c.Assert(err, check.IsNil)
	err = vs.storage.RenameTeam(context.TODO(), "myteam", "mynewteam")
	c.Assert(err, check.IsNil)
	vols, err := vs.ListByFilter(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	sort.Slice(vols, func(i, j int) bool { return vols[i].Name < vols[j].Name })
	c.Assert(vols[0].TeamOwner, check.Equals, "mynewteam")
	c.Assert(vols[1].TeamOwner, check.Equals, "otherteam")
}
