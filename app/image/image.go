// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
)

func ImageHistorySize() int {
	imgHistorySize, _ := config.GetInt("docker:image-history-size")
	if imgHistorySize == 0 {
		imgHistorySize = 10
	}
	return imgHistorySize
}

func SplitImageName(imageName string) (repo, tag string) {
	imgNameSplit := strings.Split(imageName, ":")
	switch len(imgNameSplit) {
	case 1:
		repo = imgNameSplit[0]
		tag = "latest"
	case 2:
		if strings.Contains(imgNameSplit[1], "/") {
			repo = imageName
			tag = "latest"
		} else {
			repo = imgNameSplit[0]
			tag = imgNameSplit[1]
		}
	default:
		repo = strings.Join(imgNameSplit[:len(imgNameSplit)-1], ":")
		tag = imgNameSplit[len(imgNameSplit)-1]
	}
	return
}

func AppBasicImageName(appName string) string {
	return fmt.Sprintf("%s/app-%s", basicImageName("tsuru"), appName)
}

func AppBuildImageName(appName, tag, team string, version int) string {
	if tag == "" {
		tag = fmt.Sprintf("v%d-builder", version)
	}
	return fmt.Sprintf("%s:%s", appBasicBuilderImageName(appName, team), tag)
}

func appBasicBuilderImageName(appName, teamName string) string {
	if teamName == "" {
		teamName = "tsuru"
	}
	return fmt.Sprintf("%s/app-%s", basicImageName(teamName), appName)
}

func basicImageName(repoName string) string {
	parts := make([]string, 0, 2)
	registry, _ := config.GetString("docker:registry")
	if registry != "" {
		parts = append(parts, registry)
	}
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	if repoNamespace == "" {
		repoNamespace = repoName
	}
	parts = append(parts, repoNamespace)
	return strings.Join(parts, "/")
}

// GetBuildImage returns the image name from app or plaftorm.
// the platform image will be returned if:
// * there are no containers;
// * the container have an empty image name;
// * the deploy number is multiple of 10.
// in all other cases the app image name will be returned.
func GetBuildImage(app appTypes.App) (string, error) {
	if usePlatformImage(app) {
		return getPlatformImage(app)
	}
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(app)
	if err != nil {
		return getPlatformImage(app)
	}
	return version.VersionInfo().DeployImage, nil
}

func usePlatformImage(app appTypes.App) bool {
	maxLayers, _ := config.GetUint("docker:max-layers")
	if maxLayers == 0 {
		maxLayers = 10
	}
	deploys := app.GetDeploys()
	return deploys%maxLayers == 0 || app.GetUpdatePlatform()
}

func getPlatformImage(app appTypes.App) (string, error) {
	version := app.GetPlatformVersion()
	if version != "latest" {
		return servicemanager.PlatformImage.FindImage(app.GetPlatform(), version)
	}
	return servicemanager.PlatformImage.CurrentImage(app.GetPlatform())
}
