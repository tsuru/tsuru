// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

var (
	ErrNoVersionsAvailable    = errors.New("no versions available for app")
	ErrVersionMarkedToRemoval = errors.New("the selected version is marked to removal")
)

type ErrInvalidVersion struct {
	Version string
}

func (i ErrInvalidVersion) Error() string {
	return fmt.Sprintf("Invalid version: %s", i.Version)
}

type App interface {
	GetName() string
	GetTeamOwner() string
	GetPlatform() string
	GetPlatformVersion() string
	GetDeploys() uint
	GetUpdatePlatform() bool
}

type AppVersion interface {
	Version() int
	BuildImageName() string
	CommitBuildImage() error
	BaseImageName() string
	CommitBaseImage() error
	CommitSuccessful() error
	MarkToRemoval() error
	VersionInfo() AppVersionInfo
	Processes() (map[string][]string, error)
	TsuruYamlData() (provTypes.TsuruYamlData, error)
	WebProcess() (string, error)
	AddData(AddVersionDataArgs) error
	String() string
	ToggleEnabled(enabled bool, reason string) error
}

type AddVersionDataArgs struct {
	Processes    map[string][]string
	CustomData   map[string]interface{}
	ExposedPorts []string
}

type AppVersions struct {
	AppName               string                 `json:"appName"`
	Count                 int                    `json:"count"`
	LastSuccessfulVersion int                    `json:"lastSuccessfulVersion"`
	Versions              map[int]AppVersionInfo `json:"versions"`
	UpdatedAt             time.Time              `json:"updatedAt"`
}

type AppVersionInfo struct {
	Version          int                    `json:"version"`
	Description      string                 `json:"description"`
	BuildImage       string                 `json:"buildImage"`
	DeployImage      string                 `json:"deployImage"`
	CustomBuildTag   string                 `json:"customBuildTag"`
	CustomData       map[string]interface{} `json:"customData"`
	Processes        map[string][]string    `json:"processes"`
	ExposedPorts     []string               `json:"exposedPorts"`
	EventID          string                 `json:"eventID"`
	CreatedAt        time.Time              `json:"createdAt"`
	UpdatedAt        time.Time              `json:"updatedAt"`
	DisabledReason   string                 `json:"disabledReason"`
	Disabled         bool                   `json:"disabled"`
	DeploySuccessful bool                   `json:"deploySuccessful"`
	MarkedToRemoval  bool                   `json:"markedToRemoval"`
}

type NewVersionArgs struct {
	EventID        string
	App            App
	CustomBuildTag string
	Description    string
}

type AppVersionService interface {
	AllAppVersions() ([]AppVersions, error)
	AppVersions(app App) (AppVersions, error)
	VersionByPendingImage(app App, imageID string) (AppVersion, error)
	VersionByImageOrVersion(app App, image string) (AppVersion, error)
	LatestSuccessfulVersion(app App) (AppVersion, error)
	NewAppVersion(args NewVersionArgs) (AppVersion, error)
	DeleteVersions(appName string) error
	DeleteVersion(appName string, version int) error
	AppVersionFromInfo(App, AppVersionInfo) AppVersion
}

type AppVersionStorage interface {
	UpdateVersion(appName string, vi *AppVersionInfo) error
	UpdateVersionSuccess(appName string, vi *AppVersionInfo) error
	NewAppVersion(args NewVersionArgs) (*AppVersionInfo, error)
	DeleteVersions(appName string) error
	AllAppVersions() ([]AppVersions, error)
	AppVersions(app App) (AppVersions, error)
	DeleteVersion(appName string, version int) error
}
