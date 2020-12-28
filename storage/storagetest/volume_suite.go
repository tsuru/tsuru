// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	"github.com/tsuru/tsuru/types/volume"
	check "gopkg.in/check.v1"
)

type VolumeSuite struct {
	SuiteHooks
	VolumeStorage volume.VolumeStorage
}

func (s *VolumeSuite) Test_SaveGetDelete(c *check.C) {
	vol := &volume.Volume{
		Name:   "my-volume",
		Pool:   "my-pool",
		Status: "provisioned",
		Opts: map[string]string{
			"opt1": "value1",
		},
		Plan: volume.VolumePlan{
			Name: "plan1",
			Opts: map[string]interface{}{
				"opt1": "value1",
			},
		},
	}
	err := s.VolumeStorage.Save(context.TODO(), vol)
	c.Assert(err, check.IsNil)
	volInDB, err := s.VolumeStorage.Get(context.TODO(), "my-volume")
	c.Assert(err, check.IsNil)
	c.Assert(volInDB, check.DeepEquals, vol)

	err = s.VolumeStorage.Delete(context.TODO(), vol)
	c.Assert(err, check.IsNil)

	_, err = s.VolumeStorage.Get(context.TODO(), "my-volume")
	c.Assert(err, check.Equals, volume.ErrVolumeNotFound)
}

func (s *VolumeSuite) Test_ListByFilter(c *check.C) {
	vols := []volume.Volume{
		{
			Name:   "my-volume1",
			Pool:   "my-pool",
			Status: "provisioned",
			Opts: map[string]string{
				"opt1": "value1",
			},
			Plan: volume.VolumePlan{
				Name: "plan1",
				Opts: map[string]interface{}{
					"opt1": "value1",
				},
			},
		},
		{
			Name:   "my-volume2",
			Pool:   "other-pool",
			Status: "provisioned",
			Opts: map[string]string{
				"opt1": "value1",
			},
			Plan: volume.VolumePlan{
				Name: "plan1",
				Opts: map[string]interface{}{
					"opt1": "value1",
				},
			},
		},
		{
			Name:      "my-volume3",
			Pool:      "other-pool",
			Status:    "provisioned",
			TeamOwner: "tsuru",
			Opts: map[string]string{
				"opt1": "value1",
			},
			Plan: volume.VolumePlan{
				Name: "plan1",
				Opts: map[string]interface{}{
					"opt1": "value1",
				},
			},
		},
	}
	for _, vol := range vols {
		err := s.VolumeStorage.Save(context.TODO(), &vol)
		c.Assert(err, check.IsNil)
	}

	asserts := []struct {
		filter          *volume.Filter
		expectedVolumes []volume.Volume
	}{
		{
			filter:          &volume.Filter{Names: []string{"my-volume1"}},
			expectedVolumes: []volume.Volume{vols[0]},
		},
		{
			filter:          &volume.Filter{Pools: []string{"other-pool"}},
			expectedVolumes: []volume.Volume{vols[1], vols[2]},
		},
		{
			filter:          &volume.Filter{Teams: []string{"tsuru"}},
			expectedVolumes: []volume.Volume{vols[2]},
		},
		{
			filter:          nil,
			expectedVolumes: vols,
		},
	}

	for _, assert := range asserts {
		volsInDB, err := s.VolumeStorage.ListByFilter(context.TODO(), assert.filter)
		c.Assert(err, check.IsNil)
		c.Assert(volsInDB, check.DeepEquals, assert.expectedVolumes)
	}
}

func (s *VolumeSuite) Test_InsertBinds(c *check.C) {
	binds := []volume.VolumeBind{
		{
			ID: volume.VolumeBindID{
				App:        "my-app",
				Volume:     "my-volume",
				MountPoint: "/mnt",
			},
			ReadOnly: true,
		},
		{
			ID: volume.VolumeBindID{
				App:        "my-app",
				Volume:     "my-volume2",
				MountPoint: "/mnt2",
			},
			ReadOnly: false,
		},
		{
			ID: volume.VolumeBindID{
				App:        "other-app",
				Volume:     "my-volume3",
				MountPoint: "/mnt",
			},
			ReadOnly: false,
		},
	}

	for _, bind := range binds {
		err := s.VolumeStorage.InsertBind(context.TODO(), &bind)
		c.Assert(err, check.IsNil)
	}

	err := s.VolumeStorage.InsertBind(context.TODO(), &binds[0])
	c.Assert(err, check.Equals, volume.ErrVolumeAlreadyBound)

	bindsInDB, err := s.VolumeStorage.Binds(context.TODO(), "my-volume")
	c.Assert(err, check.IsNil)
	c.Assert(bindsInDB, check.DeepEquals, binds[0:1])

	bindsInDB, err = s.VolumeStorage.Binds(context.TODO(), "my-volume2")
	c.Assert(err, check.IsNil)
	c.Assert(bindsInDB, check.DeepEquals, binds[1:2])

	bindsInDB, err = s.VolumeStorage.BindsForApp(context.TODO(), "", "my-app")
	c.Assert(err, check.IsNil)
	c.Assert(bindsInDB, check.DeepEquals, binds[0:2])

	bindsInDB, err = s.VolumeStorage.BindsForApp(context.TODO(), "my-volume2", "my-app")
	c.Assert(err, check.IsNil)
	c.Assert(bindsInDB, check.DeepEquals, binds[1:2])

	err = s.VolumeStorage.RemoveBind(context.TODO(), binds[1].ID)
	c.Assert(err, check.IsNil)

	bindsInDB, err = s.VolumeStorage.Binds(context.TODO(), "my-volume2")
	c.Assert(err, check.IsNil)
	c.Assert(bindsInDB, check.HasLen, 0)
}

func (s *VolumeSuite) Test_RenameTeam(c *check.C) {
	vol := &volume.Volume{
		Name:      "my-volume",
		Pool:      "my-pool",
		Status:    "provisioned",
		TeamOwner: "tsuru",
		Opts: map[string]string{
			"opt1": "value1",
		},
		Plan: volume.VolumePlan{
			Name: "plan1",
			Opts: map[string]interface{}{
				"opt1": "value1",
			},
		},
	}
	err := s.VolumeStorage.Save(context.TODO(), vol)
	c.Assert(err, check.IsNil)
	err = s.VolumeStorage.RenameTeam(context.TODO(), "tsuru", "kubernetes")
	c.Assert(err, check.IsNil)
	volInDB, err := s.VolumeStorage.Get(context.TODO(), "my-volume")
	c.Assert(err, check.IsNil)
	c.Assert(volInDB.TeamOwner, check.Equals, "kubernetes")
}
