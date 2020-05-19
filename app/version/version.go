// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

type appVersionImpl struct {
	storage     appTypes.AppVersionStorage
	app         appTypes.App
	versionInfo *appTypes.AppVersionInfo
}

var _ appTypes.AppVersion = &appVersionImpl{}

func (v *appVersionImpl) BuildImageName() string {
	return image.AppBuildImageName(v.app.GetName(), v.versionInfo.CustomBuildTag, v.app.GetTeamOwner(), v.Version())
}

func (v *appVersionImpl) CommitBuildImage() error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.BuildImage = v.BuildImageName()
	return v.storage.UpdateVersion(v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) BaseImageName() string {
	return fmt.Sprintf("%s:v%d", image.AppBasicImageName(v.app.GetName()), v.versionInfo.Version)
}

func (v *appVersionImpl) CommitBaseImage() error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.DeployImage = v.BaseImageName()
	return v.storage.UpdateVersion(v.app.GetName(), v.versionInfo)
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
		if name == "web" || len(v.versionInfo.Processes) == 1 {
			return name, nil
		}
		processes = append(processes, name)
	}
	sort.Strings(processes)
	if len(processes) > 0 {
		return processes[0], nil
	}
	return "", nil
}

func (v *appVersionImpl) CommitSuccessful() error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.DeploySuccessful = true
	return v.storage.UpdateVersionSuccess(v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) MarkToRemoval() error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.MarkedToRemoval = true
	return v.storage.UpdateVersion(v.app.GetName(), v.versionInfo)
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
	return v.storage.UpdateVersion(v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) ToggleEnabled(enabled bool, reason string) error {
	err := v.refresh()
	if err != nil {
		return err
	}
	v.versionInfo.Disabled = !enabled
	v.versionInfo.DisabledReason = reason
	return v.storage.UpdateVersion(v.app.GetName(), v.versionInfo)
}

func (v *appVersionImpl) Version() int {
	return v.VersionInfo().Version
}

func (v *appVersionImpl) String() string {
	return fmt.Sprintf("(version %d, buildImage %s, deployImage %s)", v.versionInfo.Version, v.versionInfo.BuildImage, v.versionInfo.DeployImage)
}

func (v *appVersionImpl) refresh() error {
	versions, err := v.storage.AppVersions(v.app)
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
