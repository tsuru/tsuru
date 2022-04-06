// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

var (
	ErrNoVersionsAvailable          = errors.New("no versions available for app")
	ErrTransactionCancelledByChange = errors.New("The update has been cancelled by a previous change")
	ErrVersionMarkedToRemoval       = errors.New("the selected version is marked to removal")
)

type ErrInvalidVersion struct {
	Version string
}

func (i ErrInvalidVersion) Error() string {
	return fmt.Sprintf("Invalid version: %s", i.Version)
}

func IsInvalidVersionError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(ErrInvalidVersion)
	return ok
}

type AppVersion interface {
	Version() int
	BuildImageName() (string, error)
	CommitBuildImage() error
	BaseImageName() (string, error)
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
	UpdatePastUnits(process string, replicas int) error
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
	UpdatedHash           string                 `json:"updatedHash"`
	MarkedToRemoval       bool                   `json:"markedToRemoval"`
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
	PastUnits        map[string]int         `json:"pastUnits"`
}

type NewVersionArgs struct {
	EventID        string
	App            App
	CustomBuildTag string
	Description    string
}

type AppVersionWriteOptions struct {
	// PreviousUpdatedHash is used to avoid a race of updates and loss data by concurrent updates.
	PreviousUpdatedHash string
}

type AppVersionService interface {
	commonAppVersion
	VersionByPendingImage(ctx context.Context, app App, imageID string) (AppVersion, error)
	VersionByImageOrVersion(ctx context.Context, app App, image string) (AppVersion, error)
	LatestSuccessfulVersion(ctx context.Context, app App) (AppVersion, error)
	NewAppVersion(ctx context.Context, args NewVersionArgs) (AppVersion, error)
	AppVersionFromInfo(context.Context, App, AppVersionInfo) (AppVersion, error)
}

type AppVersionStorage interface {
	commonAppVersion
	UpdateVersion(ctx context.Context, appName string, vi *AppVersionInfo, opts ...*AppVersionWriteOptions) error
	UpdateVersionSuccess(ctx context.Context, appName string, vi *AppVersionInfo, opts ...*AppVersionWriteOptions) error
	NewAppVersion(ctx context.Context, args NewVersionArgs) (*AppVersionInfo, error)
}

type commonAppVersion interface {
	AllAppVersions(ctx context.Context, appNamesFilter ...string) ([]AppVersions, error)
	AppVersions(ctx context.Context, app App) (AppVersions, error)
	DeleteVersions(ctx context.Context, appName string, opts ...*AppVersionWriteOptions) error
	DeleteVersionIDs(ctx context.Context, appName string, versions []int, opts ...*AppVersionWriteOptions) error
	MarkToRemoval(ctx context.Context, appName string, opts ...*AppVersionWriteOptions) error
	MarkVersionsToRemoval(ctx context.Context, appName string, versions []int, opts ...*AppVersionWriteOptions) error
}
