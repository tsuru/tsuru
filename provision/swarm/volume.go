// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"strings"

	"github.com/docker/docker/api/types/mount"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/volume"
)

type volumeOptions struct {
	Driver string
	Opts   []string
}

func mountsForApp(app provision.App) ([]mount.Mount, error) {
	volumes, err := volume.ListByApp(app.GetName())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var mounts []mount.Mount
	for _, v := range volumes {
		m, err := mountsForVolume(v, app)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, m...)
	}
	return mounts, nil
}

func mountsForVolume(v volume.Volume, app provision.App) ([]mount.Mount, error) {
	var volumeOpts volumeOptions
	err := v.UnmarshalPlan(&volumeOpts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	optsMap := make(map[string]string)
	for k, v := range v.Opts {
		optsMap[k] = v
	}
	for _, o := range volumeOpts.Opts {
		parts := strings.SplitN(o, "=", 2)
		if len(parts) != 2 {
			return nil, errors.New("driver option must be on the form of key=value")
		}
		optsMap[parts[0]] = parts[1]
	}
	binds, err := v.LoadBindsForApp(app.GetName())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	labelSet := provision.VolumeLabels(provision.VolumeLabelsOpts{
		Name:        v.Name,
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
		Pool:        v.Pool,
		Plan:        v.Plan.Name,
		Team:        v.TeamOwner,
	})
	var mounts []mount.Mount
	for _, b := range binds {
		m := mount.Mount{
			Source:   v.Name,
			Target:   b.ID.MountPoint,
			ReadOnly: b.ReadOnly,
			Type:     mount.TypeVolume,
			VolumeOptions: &mount.VolumeOptions{
				Labels: labelSet.ToLabels(),
				DriverConfig: &mount.Driver{
					Name:    volumeOpts.Driver,
					Options: optsMap,
				},
			},
		}
		mounts = append(mounts, m)
	}
	return mounts, nil
}
