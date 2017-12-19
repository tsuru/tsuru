// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types"
	"gopkg.in/check.v1"
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

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func updateConfig(data string) {
	config.ReadConfigBytes([]byte(data))
	config.Set("database:url", "127.0.0.1:27017")
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
}

func (p *volumeProvisioner) GetName() string {
	return "volumeprov"
}

func (p *volumeProvisioner) DeleteVolume(volName, pool string) error {
	p.deleteCallVolume = volName
	p.deleteCallPool = pool
	return nil
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
	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "mypool",
		Provisioner: "fake",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{
		Name:        "otherpool",
		Provisioner: "fake",
	})
	c.Assert(err, check.IsNil)
	err = auth.TeamService().Insert(types.Team{Name: "myteam"})
	c.Assert(err, check.IsNil)
	err = auth.TeamService().Insert(types.Team{Name: "otherteam"})
	c.Assert(err, check.IsNil)
	updateConfig(baseConfig)
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Volumes().Database.DropDatabase()
	c.Assert(err, check.IsNil)
}

func (s *S) TestVolumeUnmarshalPlan(c *check.C) {
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
	err := pool.AddPool(pool.AddPoolOptions{
		Name:        "mypool2",
		Provisioner: "other",
	})
	c.Assert(err, check.IsNil)
	v1 := Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam", Plan: VolumePlan{Name: "nfs"}}
	err = v1.Validate()
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
	err = v1.Validate()
	c.Assert(err, check.IsNil)
	resultFake = fakePlan{}
	err = v1.UnmarshalPlan(&resultFake)
	c.Assert(err, check.IsNil)
	c.Assert(resultFake, check.DeepEquals, fakePlan{
		Driver: "rexray/ebs",
	})
	v1.Plan.Name = "nfs"
	v1.Pool = "mypool2"
	err = v1.Validate()
	c.Assert(err, check.IsNil)
	var resultFakeOther fakeOtherPlan
	err = v1.UnmarshalPlan(&resultFakeOther)
	c.Assert(err, check.IsNil)
	c.Assert(resultFakeOther, check.DeepEquals, fakeOtherPlan{
		Plugin: "nfs",
	})
	v1.Plan.Name = "ebs"
	err = v1.Validate()
	c.Assert(err, check.IsNil)
	resultFakeOther = fakeOtherPlan{}
	err = v1.UnmarshalPlan(&resultFakeOther)
	c.Assert(err, check.IsNil)
	c.Assert(resultFakeOther, check.DeepEquals, fakeOtherPlan{
		StorageClass: "my-ebs-storage-class",
	})
}

func (s *S) TestVolumeSaveLoad(c *check.C) {
	tests := []struct {
		v   Volume
		err string
	}{
		{
			v:   Volume{},
			err: "volume name cannot be empty",
		},
		{
			v:   Volume{Name: "v1"},
			err: pool.ErrPoolNotFound.Error(),
		},
		{
			v:   Volume{Name: "v1", Pool: "mypool"},
			err: types.ErrTeamNotFound.Error(),
		},
		{
			v:   Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam"},
			err: "key \"volume-plans::fake\" not found",
		},
		{
			v:   Volume{Name: "v1", Pool: "mypool", TeamOwner: "myteam", Plan: VolumePlan{Name: "bogus"}},
			err: "key \"volume-plans:bogus:fake\" not found",
		},
		{
			v: Volume{
				Name: "v1",
				Plan: VolumePlan{
					Name: "p1",
				},
				Pool:      "mypool",
				TeamOwner: "myteam",
			},
		},
		{
			v: Volume{
				Name: "v1",
				Plan: VolumePlan{
					Name: "p1",
				},
				Pool:      "mypool",
				TeamOwner: "myteam",
				Opts:      map[string]string{"opt1": "val1"},
			},
		},
	}
	for i, tt := range tests {
		err := tt.v.Save()
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
		dbV, err := Load(tt.v.Name)
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
	v := Volume{
		Name:      "v1",
		Plan:      VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp("myapp", "/mnt1", true)
	c.Assert(err, check.IsNil)
	binds, err := v.LoadBinds()
	c.Assert(err, check.IsNil)
	expected := []VolumeBind{{ID: VolumeBindID{App: "myapp", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: true}}
	c.Assert(binds, check.DeepEquals, expected)
	dbV, err := Load(v.Name)
	c.Assert(err, check.IsNil)
	binds, err = dbV.LoadBinds()
	c.Assert(err, check.IsNil)
	c.Assert(binds, check.DeepEquals, expected)
}

func (s *S) TestVolumeBindAppMultipleMounts(c *check.C) {
	v := Volume{
		Name:      "v1",
		Plan:      VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp("myapp", "/mnt1", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp("myapp", "/mnt2", true)
	c.Assert(err, check.IsNil)
	err = v.BindApp("myapp", "/mnt2", false)
	c.Assert(err, check.Equals, ErrVolumeAlreadyBound)
	expected := []VolumeBind{
		{ID: VolumeBindID{App: "myapp", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: VolumeBindID{App: "myapp", MountPoint: "/mnt2", Volume: "v1"}, ReadOnly: true},
	}
	binds, err := v.LoadBinds()
	c.Assert(err, check.IsNil)
	c.Assert(binds, check.DeepEquals, expected)
}

func (s *S) TestVolumeUnbindApp(c *check.C) {
	v := Volume{
		Name:      "v1",
		Plan:      VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp("myapp", "/mnt1", true)
	c.Assert(err, check.IsNil)
	err = v.BindApp("myapp", "/mnt2", true)
	c.Assert(err, check.IsNil)
	err = v.UnbindApp("myapp", "/mnt1")
	c.Assert(err, check.IsNil)
	binds, err := v.LoadBinds()
	c.Assert(err, check.IsNil)
	expected := []VolumeBind{{ID: VolumeBindID{App: "myapp", MountPoint: "/mnt2", Volume: "v1"}, ReadOnly: true}}
	c.Assert(binds, check.DeepEquals, expected)
	err = v.UnbindApp("myapp", "/mnt999")
	c.Assert(err, check.Equals, ErrVolumeBindNotFound)
}

func (s *S) TestListByApp(c *check.C) {
	volumes := []Volume{
		{
			Name:      "v1",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v2",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v3",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
		},
	}
	binds := []VolumeBind{
		{ID: VolumeBindID{App: "app1", MountPoint: "/mnt1", Volume: "v1"}, ReadOnly: false},
		{ID: VolumeBindID{App: "app1", MountPoint: "/mnt2", Volume: "v1"}, ReadOnly: false},
		{ID: VolumeBindID{App: "app2", MountPoint: "/mnt1", Volume: "v2"}, ReadOnly: false},
		{ID: VolumeBindID{App: "app1", MountPoint: "/mnt1", Volume: "v3"}, ReadOnly: false},
		{ID: VolumeBindID{App: "app3", MountPoint: "/mnt1", Volume: "v3"}, ReadOnly: false},
	}
	for i, v := range volumes {
		err := v.Save()
		c.Assert(err, check.IsNil)
		volumes[i].Plan.Opts = map[string]interface{}{
			"driver": "local",
			"opt": map[string]interface{}{
				"type": "nfs",
			},
		}
		for _, b := range binds {
			if b.ID.Volume == v.Name {
				err := v.BindApp(b.ID.App, b.ID.MountPoint, b.ReadOnly)
				c.Assert(err, check.IsNil)
			}
		}
	}
	appVolumes, err := ListByApp("app1")
	c.Assert(err, check.IsNil)
	sort.Slice(appVolumes, func(i, j int) bool { return appVolumes[i].Name < appVolumes[j].Name })
	c.Assert(appVolumes, check.DeepEquals, []Volume{volumes[0], volumes[2]})
	appVolumes, err = ListByApp("app2")
	c.Assert(err, check.IsNil)
	c.Assert(appVolumes, check.DeepEquals, []Volume{volumes[1]})
	appVolumes, err = ListByApp("app3")
	c.Assert(err, check.IsNil)
	c.Assert(appVolumes, check.DeepEquals, []Volume{volumes[2]})
	appVolumes, err = ListByApp("app4")
	c.Assert(err, check.IsNil)
	c.Assert(appVolumes, check.IsNil)
}

func (s *S) TestVolumeDelete(c *check.C) {
	v := Volume{
		Name:      "v1",
		Plan:      VolumePlan{Name: "p1"},
		Pool:      "mypool",
		TeamOwner: "myteam",
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.Delete()
	c.Assert(err, check.IsNil)
	_, err = Load(v.Name)
	c.Assert(err, check.Equals, ErrVolumeNotFound)
}

func (s *S) TestVolumeDeleteWithVolumeProvisioner(c *check.C) {
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
	err := pool.AddPool(pool.AddPoolOptions{
		Name:        "volumepool",
		Provisioner: "volumeprov",
	})
	c.Assert(err, check.IsNil)
	v := Volume{
		Name:      "v1",
		Plan:      VolumePlan{Name: "p1"},
		Pool:      "volumepool",
		TeamOwner: "myteam",
	}
	err = v.Save()
	c.Assert(err, check.IsNil)
	err = v.Delete()
	c.Assert(err, check.IsNil)
	c.Assert(volumeProv.deleteCallVolume, check.Equals, "v1")
	c.Assert(volumeProv.deleteCallPool, check.Equals, "volumepool")
	_, err = Load(v.Name)
	c.Assert(err, check.Equals, ErrVolumeNotFound)
}

func (s *S) TestListByFilter(c *check.C) {
	volumes := []Volume{
		{
			Name:      "v1",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v2",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "otherpool",
			TeamOwner: "myteam",
		},
		{
			Name:      "v3",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "otherteam",
		},
	}
	for i, v := range volumes {
		err := v.Save()
		c.Assert(err, check.IsNil)
		volumes[i].Plan.Opts = map[string]interface{}{
			"driver": "local",
			"opt": map[string]interface{}{
				"type": "nfs",
			},
		}
	}
	tests := []struct {
		filter   *Filter
		expected []Volume
	}{
		{filter: nil, expected: volumes},
		{filter: &Filter{Names: []string{"v1", "v2"}}, expected: volumes[:2]},
		{filter: &Filter{Names: []string{"v1", "vx"}}, expected: volumes[:1]},
		{filter: &Filter{Names: []string{"v1", "vx"}, Teams: []string{"myteam"}}, expected: volumes[:2]},
		{filter: &Filter{Names: []string{"v1", "vx"}, Pools: []string{"otherpool"}}, expected: volumes[:2]},
		{filter: &Filter{Pools: []string{"otherpool", "mypool"}}, expected: volumes},
	}
	for _, tt := range tests {
		vols, err := ListByFilter(tt.filter)
		c.Assert(err, check.IsNil)
		sort.Slice(vols, func(i, j int) bool { return vols[i].Name < vols[j].Name })
		c.Assert(vols, check.DeepEquals, tt.expected)
	}
}

func (s *S) TestVolumeValidate(c *check.C) {
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
	msg := "Invalid volume name, volume name should have at most 63 " +
		"characters, containing only lower case letters, numbers or dashes, " +
		"starting with a letter."
	nameErr := &tsuruErrors.ValidationError{Message: msg}
	tt := []struct {
		volume      Volume
		expectedErr error
	}{
		{Volume{Name: "volume1", Pool: "mypool", TeamOwner: "myteam", Plan: VolumePlan{Name: "nfs"}}, nil},
		{Volume{Name: "volume_1", Pool: "mypool", TeamOwner: "myteam", Plan: VolumePlan{Name: "nfs"}}, nameErr},
		{Volume{Name: "123volume", Pool: "mypool", TeamOwner: "myteam", Plan: VolumePlan{Name: "nfs"}}, nameErr},
		{Volume{Name: "volume1", Pool: "invalidpool", TeamOwner: "myteam", Plan: VolumePlan{Name: "nfs"}}, pool.ErrPoolNotFound},
		{Volume{Name: "volume1", Pool: "mypool", TeamOwner: "invalidteam", Plan: VolumePlan{Name: "nfs"}}, types.ErrTeamNotFound},
		{Volume{Name: "volume1", Pool: "mypool", TeamOwner: "myteam", Plan: VolumePlan{Name: "invalidplan"}}, config.ErrKeyNotFound{Key: "volume-plans:invalidplan:fake"}},
	}
	for _, t := range tt {
		c.Assert(errors.Cause(t.volume.Validate()), check.DeepEquals, t.expectedErr, check.Commentf(t.volume.Name))
	}
}

func (s *S) TestRenameTeam(c *check.C) {
	v1 := Volume{Name: "v1", Plan: VolumePlan{Name: "p1"}, Pool: "mypool", TeamOwner: "myteam"}
	err := v1.Save()
	c.Assert(err, check.IsNil)
	v2 := Volume{Name: "v2", Plan: VolumePlan{Name: "p1"}, Pool: "mypool", TeamOwner: "otherteam"}
	err = v2.Save()
	c.Assert(err, check.IsNil)
	err = RenameTeam("myteam", "mynewteam")
	c.Assert(err, check.IsNil)
	vols, err := ListByFilter(nil)
	c.Assert(err, check.IsNil)
	sort.Slice(vols, func(i, j int) bool { return vols[i].Name < vols[j].Name })
	c.Assert(vols[0].TeamOwner, check.Equals, "mynewteam")
	c.Assert(vols[1].TeamOwner, check.Equals, "otherteam")
}
