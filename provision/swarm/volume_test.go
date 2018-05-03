// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"github.com/docker/docker/api/types/mount"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/volume"
	check "gopkg.in/check.v1"
)

func (s *S) TestMountsForApp(c *check.C) {
	config.Set("volume-plans:p1:swarm:driver", "local")
	config.Set("volume-plans:p1:swarm:opts", []string{"type=nfs"})
	defer config.Unset("volume-plans")
	a := provisiontest.NewFakeApp("myapp", "pyton", "test-default", 0)
	v := volume.Volume{
		Name: "v1",
		Opts: map[string]string{
			"device": "/exports",
			"o":      "addr=192.168.50.4,rw",
		},
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      "bonehunters",
		TeamOwner: "admin",
	}
	err := v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt2", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp("otherapp", "/mnt", false)
	c.Assert(err, check.IsNil)
	mounts, err := mountsForApp(a)
	c.Assert(err, check.IsNil)
	c.Assert(mounts, check.DeepEquals, []mount.Mount{
		{
			Source:   v.Name,
			Target:   "/mnt",
			ReadOnly: false,
			Type:     mount.TypeVolume,
			VolumeOptions: &mount.VolumeOptions{
				Labels: map[string]string{
					"tsuru.is-tsuru":    "true",
					"tsuru.provisioner": "swarm",
					"tsuru.volume-name": "v1",
					"tsuru.volume-plan": "p1",
					"tsuru.volume-pool": "bonehunters",
				},
				DriverConfig: &mount.Driver{
					Name: "local",
					Options: map[string]string{
						"type":   "nfs",
						"device": "/exports",
						"o":      "addr=192.168.50.4,rw",
					},
				},
			},
		},
		{
			Source:   v.Name,
			Target:   "/mnt2",
			ReadOnly: false,
			Type:     mount.TypeVolume,
			VolumeOptions: &mount.VolumeOptions{
				Labels: map[string]string{
					"tsuru.is-tsuru":    "true",
					"tsuru.provisioner": "swarm",
					"tsuru.volume-name": "v1",
					"tsuru.volume-plan": "p1",
					"tsuru.volume-pool": "bonehunters",
				},
				DriverConfig: &mount.Driver{
					Name: "local",
					Options: map[string]string{
						"type":   "nfs",
						"device": "/exports",
						"o":      "addr=192.168.50.4,rw",
					},
				},
			},
		},
	})
}
