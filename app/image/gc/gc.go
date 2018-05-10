// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/registry"
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

func CleanImage(appName string, imgName string, removeFromRegistry bool) {
	a, err := app.GetByName(appName)
	if err != nil {
		log.Errorf("[image gc] error getting app by name %q: %v. Image kept on list to retry later.",
			imgName, err.Error())
		return
	}
	cleanImageForApp(a, appName, imgName, removeFromRegistry)
}

func cleanImageForApp(a *app.App, appName string, imgName string, removeFromRegistry bool) {
	shouldRemove := true
	defer func() {
		log.Debugf("[image gc] image %q processed, removed from registry: %v, removed from database: %v", imgName, removeFromRegistry, removeFromRegistry && shouldRemove)
	}()
	err := a.CleanImage(imgName)
	if err != nil {
		shouldRemove = false
		log.Errorf("[image gc] error removing old image from provisioner for app %q: %v. Image kept on list to retry later.",
			imgName, err.Error())
	}
	if removeFromRegistry {
		if err := registry.RemoveImageIgnoreNotFound(imgName); err != nil {
			shouldRemove = false
			log.Errorf("[image gc] error removing old image from registry %q: %s. Image kept on list to retry later.",
				imgName, err.Error())
			return
		}
		if shouldRemove {
			if err := image.PullAppImageNames(appName, []string{imgName}); err != nil {
				log.Errorf("[image gc] error pulling old images from database %q: %s", imgName, err)
			}
		}
	}
}

func removeOldImages() error {
	log.Debugf("[image gc] starting image gc process")
	defer log.Debugf("[image gc] finished image gc process")
	allAppImages, err := image.ListAllAppImages()
	if err != nil {
		return err
	}
	historySize := image.ImageHistorySize()
	multi := tsuruErrors.NewMultiError()
	for appName, appImages := range allAppImages {
		log.Debugf("[image gc] processing %d images for app %q", len(appImages.BuilderImages)+len(appImages.DeployImages), appName)
		a, err := app.GetByName(appName)
		if err != nil && err != appTypes.ErrAppNotFound {
			multi.Add(err)
			continue
		}
		if a == nil {
			log.Debugf("[image gc] app %q not found, removing everything", appName)
			err = registry.RemoveAppImages(appName)
			if err != nil {
				multi.Add(err)
			}
			err = image.DeleteAllAppImageNames(appName)
			if err != nil {
				multi.Add(err)
			}
			continue
		}
		limit := len(appImages.DeployImages) - historySize
		for i, imgName := range appImages.DeployImages {
			if i == len(appImages.DeployImages)-1 {
				continue
			}
			cleanImageForApp(a, appName, imgName, i < limit)
		}
		builderLimit := len(appImages.BuilderImages) - historySize
		for i, imgName := range appImages.BuilderImages {
			removeFromRegistry := i < builderLimit && strings.HasSuffix(imgName, "-builder")
			if i == len(appImages.BuilderImages)-1 {
				continue
			}
			cleanImageForApp(a, appName, imgName, removeFromRegistry)
		}
	}
	return multi.ToError()
}
