// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/registry"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
)

const (
	imageGCRunInterval = 5 * time.Minute
)

func Initialize() error {
	gc := &imgGC{once: &sync.Once{}}
	gc.start()
	shutdown.Register(gc)
	return nil
}

type imgGC struct {
	once   *sync.Once
	stopCh chan struct{}
}

func (g *imgGC) start() {
	g.once.Do(func() {
		g.stopCh = make(chan struct{})
		go g.spin()
	})
}

func (g *imgGC) Shutdown(ctx context.Context) error {
	if g.stopCh == nil {
		return nil
	}
	g.stopCh <- struct{}{}
	g.stopCh = nil
	g.once = &sync.Once{}
	return nil
}

func (g *imgGC) spin() {
	for {
		err := removeOldImages()
		if err != nil {
			log.Errorf("[image gc] errors running GC: %v", err)
		}
		select {
		case <-g.stopCh:
			return
		case <-time.After(imageGCRunInterval):
		}
	}
}

func CleanImage(appName string, version appTypes.AppVersionInfo, removeFromRegistry bool) {
	a, err := app.GetByName(appName)
	if err != nil {
		log.Errorf("[image gc] error getting app by name %q: %v. Image kept on list to retry later.", appName, err)
		return
	}
	cleanImageForAppVersion(a, version, removeFromRegistry)
}

func cleanImageForAppVersion(a *app.App, version appTypes.AppVersionInfo, removeFromRegistry bool) {
	shouldRemove := removeFromRegistry
	if version.DeployImage != "" {
		cleanResult := cleanImageForApp(a, version.DeployImage, removeFromRegistry)
		shouldRemove = shouldRemove && cleanResult
	}
	if version.BuildImage != "" {
		cleanResult := cleanImageForApp(a, version.BuildImage, removeFromRegistry)
		shouldRemove = shouldRemove && cleanResult
	}
	if shouldRemove {
		err := servicemanager.AppVersion.DeleteVersion(a.Name, version.Version)
		if err != nil {
			log.Errorf("[image gc] error removing old version from database %q: %s", version, err)
		}
	}
}

func cleanImageForApp(a *app.App, imgName string, removeFromRegistry bool) bool {
	shouldRemove := true
	defer func() {
		log.Debugf("[image gc] image %q processed, removed from registry: %v, removed from database: %v", imgName, removeFromRegistry, removeFromRegistry && shouldRemove)
	}()
	// after deprecation of dockercluster we can remove the call of CleanImage
	err := a.CleanImage(imgName)
	if err != nil {
		shouldRemove = false
		log.Errorf("[image gc] error removing old image from provisioner for app %q: %v. Image kept on list to retry later.",
			imgName, err.Error())
	}
	if removeFromRegistry {
		if err := registry.RemoveImageIgnoreNotFound(imgName); err != nil {
			log.Errorf("[image gc] error removing old image from registry %q: %s. Image kept on list to retry later.",
				imgName, err.Error())
			return false
		}
		return shouldRemove
	}
	return false
}

func removeOldImages() error {
	log.Debugf("[image gc] starting image gc process")
	defer log.Debugf("[image gc] finished image gc process")
	allAppVersions, err := servicemanager.AppVersion.AllAppVersions()
	if err != nil {
		return err
	}
	historySize := image.ImageHistorySize()
	multi := tsuruErrors.NewMultiError()
	for _, versions := range allAppVersions {
		log.Debugf("[image gc] processing %d versions for app %q", len(versions.Versions), versions.AppName)
		a, err := app.GetByName(versions.AppName)
		if err != nil && err != appTypes.ErrAppNotFound {
			multi.Add(err)
			continue
		}
		if a == nil {
			log.Debugf("[image gc] app %q not found, removing everything", versions.AppName)
			err = registry.RemoveAppImages(versions.AppName)
			if err != nil {
				multi.Add(err)
			}
			err = servicemanager.AppVersion.DeleteVersions(versions.AppName)
			if err != nil {
				multi.Add(err)
			}
			continue
		}
		versionsToRemove, versionsToClean := gcForAppVersions(versions, historySize)
		for _, version := range versionsToRemove {
			cleanImageForAppVersion(a, version, true)
		}
		for _, version := range versionsToClean {
			cleanImageForAppVersion(a, version, false)
		}
	}
	return multi.ToError()
}

func gcForAppVersions(versions appTypes.AppVersions, historySize int) (versionsToRemove, versionsToMaintain []appTypes.AppVersionInfo) {
	var regularVersions, customTagVersions []appTypes.AppVersionInfo
	for _, v := range versions.Versions {
		if !v.DeploySuccessful {
			versionsToRemove = append(versionsToRemove, v)
		} else if v.CustomBuildTag != "" {
			customTagVersions = append(customTagVersions, v)
		} else {
			regularVersions = append(regularVersions, v)
		}
	}

	sort.Sort(priorizedAppVersions(versionsToRemove))
	sort.Sort(priorizedAppVersions(regularVersions))
	sort.Sort(priorizedAppVersions(customTagVersions))

	runningRegularVersions := 0
	for i, version := range regularVersions {
		// never consider lastSuccessfulversion to garbage collection
		if i == 0 || version.Version == versions.LastSuccessfulVersion {
			runningRegularVersions++
			continue
		}
		if i >= historySize {
			versionsToRemove = append(versionsToRemove, version)
		} else {
			versionsToMaintain = append(versionsToMaintain, version)
		}
	}

	versionsToMaintain = append(versionsToMaintain, customTagVersions...)
	return versionsToRemove, versionsToMaintain
}

type priorizedAppVersions []appTypes.AppVersionInfo

func (p priorizedAppVersions) Len() int      { return len(p) }
func (p priorizedAppVersions) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p priorizedAppVersions) Less(i, j int) bool {
	if p[i].UpdatedAt.Equal(p[j].UpdatedAt) {
		return p[i].Version > p[j].Version
	}

	return p[i].UpdatedAt.After(p[j].UpdatedAt)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
