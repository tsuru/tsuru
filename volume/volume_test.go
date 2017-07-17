// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package volume

import (
	"sort"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
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
	err = provision.AddPool(provision.AddPoolOptions{
		Name:        "mypool",
		Provisioner: "fake",
	})
	c.Assert(err, check.IsNil)
	team := &auth.Team{Name: "myteam"}
	err = conn.Teams().Insert(team)
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
	err := provision.AddPool(provision.AddPoolOptions{
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
			err: provision.ErrPoolNotFound.Error(),
		},
		{
			v:   Volume{Name: "v1", Pool: "mypool"},
			err: auth.ErrTeamNotFound.Error(),
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
	err = v.BindApp("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(v.Apps, check.DeepEquals, []string{"myapp"})
	dbV, err := Load(v.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbV.Apps, check.DeepEquals, []string{"myapp"})
}

func (s *S) TestListByApp(c *check.C) {
	volumes := []Volume{
		{
			Name:      "v1",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
			Apps:      []string{"app1"},
		},
		{
			Name:      "v2",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
			Apps:      []string{"app2"},
		},
		{
			Name:      "v3",
			Plan:      VolumePlan{Name: "p1"},
			Pool:      "mypool",
			TeamOwner: "myteam",
			Apps:      []string{"app1", "app3"},
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
