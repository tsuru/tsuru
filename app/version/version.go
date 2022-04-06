// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

type appVersionImpl struct {
	ctx         context.Context
	storage     appTypes.AppVersionStorage
	app         appTypes.App
	versionInfo *appTypes.AppVersionInfo
	reg         imgTypes.ImageRegistry
}

func newAppVersionImpl(ctx context.Context, storage appTypes.AppVersionStorage, app appTypes.App, versionInfo *appTypes.AppVersionInfo) (*appVersionImpl, error) {
	reg, err := app.GetRegistry()
	if err != nil {
		return nil, err
	}
	return &appVersionImpl{
		ctx:         ctx,
		storage:     storage,
		app:         app,
		versionInfo: versionInfo,
		reg:         reg,
	}, nil
}

var _ appTypes.AppVersion = &appVersionImpl{}

func (v *appVersionImpl) BuildImageName() (string, error) {
	return image.AppBuildImageName(v.reg, v.app.GetName(), v.versionInfo.CustomBuildTag, v.app.GetTeamOwner(), v.Version())
}

func (v *appVersionImpl) CommitBuildImage() error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.BuildImage, err = v.BuildImageName()
	if err != nil {
		return err
	}
	return v.storage.UpdateVersion(v.ctx, v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) BaseImageName() (string, error) {
	newImage, err := image.AppBasicImageName(v.reg, v.app.GetName())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:v%d", newImage, v.versionInfo.Version), nil
}

func (v *appVersionImpl) CommitBaseImage() error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.DeployImage, err = v.BaseImageName()
	if err != nil {
		return err
	}
	return v.storage.UpdateVersion(v.ctx, v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) VersionInfo() appTypes.AppVersionInfo {
	return *v.versionInfo
}

func (v *appVersionImpl) TsuruYamlData() (provTypes.TsuruYamlData, error) {
	err := v.refresh()
	if err != nil {
		return provTypes.TsuruYamlData{}, err
	}
	return unmarshalYamlData(v.VersionInfo().CustomData)
}

func (v *appVersionImpl) Processes() (map[string][]string, error) {
	err := v.refresh()
	if err != nil {
		return nil, err
	}
	return v.VersionInfo().Processes, nil
}

func (v *appVersionImpl) WebProcess() (string, error) {
	allProcesses, err := v.Processes()
	if err != nil {
		return "", err
	}
	var processes []string
	for name := range allProcesses {
		processes = append(processes, name)
	}
	return provision.MainAppProcess(processes), nil
}

func (v *appVersionImpl) CommitSuccessful() error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.DeploySuccessful = true
	return v.storage.UpdateVersionSuccess(v.ctx, v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) MarkToRemoval() error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.MarkedToRemoval = true
	return v.storage.UpdateVersion(v.ctx, v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) AddData(args appTypes.AddVersionDataArgs) error {
	err := v.refresh()
	if err != nil {
		return err
	}
	if args.CustomData != nil {
		if args.Processes == nil {
			args.Processes, err = processesFromCustomData(args.CustomData)
			if err != nil {
				return err
			}
		}
		v.versionInfo.CustomData, err = marshalCustomData(args.CustomData)
		if err != nil {
			return err
		}
	}
	if args.Processes != nil {
		v.versionInfo.Processes = args.Processes
	}
	if args.ExposedPorts != nil {
		v.versionInfo.ExposedPorts = args.ExposedPorts
	}
	return v.storage.UpdateVersion(v.ctx, v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) UpdatePastUnits(process string, replicas int) error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.PastUnits[process] = replicas

	return v.storage.UpdateVersion(v.ctx, v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) ToggleEnabled(enabled bool, reason string) error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.Disabled = !enabled
	v.versionInfo.DisabledReason = reason
	return v.storage.UpdateVersion(v.ctx, v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) Version() int {
	return v.VersionInfo().Version
}

func (v *appVersionImpl) String() string {
	return fmt.Sprintf("(version %d, buildImage %s, deployImage %s)", v.versionInfo.Version, v.versionInfo.BuildImage, v.versionInfo.DeployImage)
}

func (v *appVersionImpl) refresh() error {
	versions, err := v.storage.AppVersions(v.ctx, v.app)
	if err != nil {
		return err
	}
	selfEntry, ok := versions.Versions[v.versionInfo.Version]
	if !ok {
		return errors.Errorf("version %d not available anymore", v.versionInfo.Version)
	}
	v.versionInfo = &selfEntry
	return nil
}
