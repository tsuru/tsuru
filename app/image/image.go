// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
)

func ImageHistorySize() int {
	imgHistorySize, _ := config.GetInt("docker:image-history-size")
	if imgHistorySize == 0 {
		imgHistorySize = 10
	}
	return imgHistorySize
}

func SplitImageName(imageName string) (repo, tag string) {
	reg, img, tag := ParseImageParts(imageName)
	if tag == "" {
		tag = "latest"
	}
	if reg != "" {
		img = strings.Join([]string{reg, img}, "/")
	}
	return img, tag
}

func ParseImageParts(imageName string) (registry string, image string, tag string) {
	parts := strings.SplitN(imageName, "/", 3)
	switch len(parts) {
	case 1:
		image = imageName
	case 2:
		if strings.ContainsAny(parts[0], ":.") || parts[0] == "localhost" {
			registry = parts[0]
			image = parts[1]
			break
		}
		image = imageName
	case 3:
		registry = parts[0]
		image = strings.Join(parts[1:], "/")
	}
	parts = strings.SplitN(image, ":", 2)
	if len(parts) < 2 {
		return registry, parts[0], ""
	}
	return registry, parts[0], parts[1]
}

func AppBasicImageName(reg imgTypes.ImageRegistry, appName string) (string, error) {
	imageName, err := basicImageName(reg, "tsuru")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/app-%s", imageName, appName), nil
}

func AppBuildImageName(reg imgTypes.ImageRegistry, appName, tag, team string, version int) (string, error) {
	if tag == "" {
		tag = fmt.Sprintf("v%d-builder", version)
	}
	imageName, err := appBasicBuilderImageName(reg, appName, team)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", imageName, tag), nil
}

func appBasicBuilderImageName(reg imgTypes.ImageRegistry, appName, teamName string) (string, error) {
	if teamName == "" {
		teamName = "tsuru"
	}
	imageName, err := basicImageName(reg, teamName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/app-%s", imageName, appName), nil
}
func resolveName(name string) (newname string, err error) {
	host, port, err := net.SplitHostPort(name)
	if err != nil {
		return "", err
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if len(addr) > 8 {
			newname = net.JoinHostPort(addr, port)
		}
	}
	return newname, nil
}
func basicImageName(reg imgTypes.ImageRegistry, repoName string) (string, error) {
	var err error
	parts := make([]string, 0, 2)
	registry := string(reg)
	if registry != "" {
		return registry, nil
	}
	registry, _ = config.GetString("docker:registry")
	resolve, _ := config.GetBool("docker:resolve-registry-name")
	if resolve {
		registry, err = resolveName(registry)
		if err != nil {
			return "", err
		}
	}
	if registry != "" {
		parts = append(parts, registry)
	}
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	if repoNamespace == "" {
		repoNamespace = repoName
	}
	parts = append(parts, repoNamespace)
	return strings.Join(parts, "/"), nil
}

// GetBuildImage returns the image name from app or plaftorm.
// the platform image will be returned if:
// * there are no containers;
// * the container have an empty image name;
// * the deploy number is multiple of 10.
// in all other cases the app image name will be returned.
func GetBuildImage(ctx context.Context, app appTypes.App) (string, error) {
	if usePlatformImage(app) {
		return getPlatformImage(ctx, app)
	}
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(ctx, app)
	if err != nil {
		return getPlatformImage(ctx, app)
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

func getPlatformImage(ctx context.Context, app appTypes.App) (string, error) {
	reg, err := app.GetRegistry()
	if err != nil {
		return "", err
	}
	version := app.GetPlatformVersion()
	if version != "latest" {
		return servicemanager.PlatformImage.FindImage(ctx, reg, app.GetPlatform(), version)
	}
	return servicemanager.PlatformImage.CurrentImage(ctx, reg, app.GetPlatform())
}
