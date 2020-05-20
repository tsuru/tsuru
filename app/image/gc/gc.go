// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsuru/config"
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

var (
	gcExecutionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_gc_executions_total",
		Help: "The number of times that gc had runned by phase",
	}, []string{"phase"})

	executionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tsuru_gc_execution_duration_seconds",
		Help:    "How long during the GC process",
		Buckets: prometheus.ExponentialBuckets(0.1, 3, 10),
	}, []string{"phase"})

	versionsMarkedToRemovalTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_gc_versions_marked_to_removal_total",
		Help: "The number of versions of applications that was marked to removal",
	})

	// just used for dockercluster provisioner
	provisionerPruneTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_gc_provisioner_prune_total",
		Help: "The number of executions of prune against the provisioner",
	})

	provisionerPruneFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_gc_provisioner_prune_failures_total",
		Help: "The number of failures to prune unused images from provisioner",
	})

	// registry metrics
	registryPruneTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_gc_registry_prune_total",
		Help: "The number of executions of prune against the registry",
	})

	registryPruneFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_gc_registry_prune_failures_total",
		Help: "The number of failures to prune unused images from registry",
	})

	// database metrics
	storagePruneTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_gc_storage_prune_total",
		Help: "The number of executions of prune against the storage",
	})

	storagePruneFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_gc_storage_prune_failures_total",
		Help: "The number of failures to prune unused images from storage",
	})
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
		err := markOldImages()
		if err != nil {
			log.Errorf("[image gc] errors running GC mark: %v", err)
		}

		dryRun, err := config.GetBool("docker:gc:dry-run")
		if err != nil {
			log.Errorf("[image gc] fetch config error: %v", err)
		}
		if !dryRun {
			err = sweepOldImages()
			if err != nil {
				log.Errorf("[image gc] errors running GC sweep: %v", err)
			}
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
	pruned := pruneVersionFromProvisioner(a, version)
	if !pruned || !removeFromRegistry {
		return
	}

	pruned = pruneVersionFromRegistry(version)
	if !pruned {
		return
	}

	pruneVersionFromStorage(appName, version)
}

func markOldImages() error {
	gcExecutionsTotal.WithLabelValues("mark").Inc()
	timer := prometheus.NewTimer(executionDuration.WithLabelValues("mark"))
	defer timer.ObserveDuration()

	log.Debugf("[image gc] starting gc process to select old images")
	defer log.Debugf("[image gc] finished gc process to select old images")
	allAppVersions, err := servicemanager.AppVersion.AllAppVersions()
	if err != nil {
		return err
	}
	historySize := image.ImageHistorySize()
	multi := tsuruErrors.NewMultiError()
	for _, appVersions := range allAppVersions {
		log.Debugf("[image gc] processing %d versions for app %q", len(appVersions.Versions), appVersions.AppName)
		a, err := app.GetByName(appVersions.AppName)
		if err != nil && err != appTypes.ErrAppNotFound {
			multi.Add(err)
			continue
		}
		if a == nil {
			log.Debugf("[image gc] app %q not found, removing everything", appVersions.AppName)
			err = registry.RemoveAppImages(appVersions.AppName)
			if err != nil {
				multi.Add(err)
			}
			err = servicemanager.AppVersion.DeleteVersions(appVersions.AppName)
			if err != nil {
				multi.Add(err)
			}
			continue
		}

		deployedVersions, err := a.DeployedVersions()
		if err == app.ErrNoVersionProvisioner {
			deployedVersions = []int{appVersions.LastSuccessfulVersion}
		} else if err != nil {
			multi.Add(errors.Wrapf(err, "Could not get deployed versions of app: %s", appVersions.AppName))
			continue
		}

		versionsToRemove, versionsToPruneFromProvisioner := selectAppVersions(appVersions, deployedVersions, historySize)
		for _, version := range versionsToRemove {
			versionsMarkedToRemovalTotal.Inc()
			pruned := pruneVersionFromProvisioner(a, version)
			if !pruned {
				continue
			}
			err = servicemanager.AppVersion.MarkVersionToRemoval(a.Name, version.Version)
			if err != nil {
				multi.Add(errors.Wrapf(err, "Could not mark version %d to removal of app: %s", version.Version, appVersions.AppName))
			}
		}
		for _, version := range versionsToPruneFromProvisioner {
			pruneVersionFromProvisioner(a, version)
		}
	}
	return multi.ToError()
}

func sweepOldImages() error {
	gcExecutionsTotal.WithLabelValues("sweep").Inc()
	timer := prometheus.NewTimer(executionDuration.WithLabelValues("sweep"))
	defer timer.ObserveDuration()

	log.Debugf("[image gc] starting gc process to sweep old images")
	defer log.Debugf("[image gc] finished gc process to sweep old images")

	allAppVersions, err := servicemanager.AppVersion.AllAppVersions()
	if err != nil {
		return err
	}

	multi := tsuruErrors.NewMultiError()
	for _, appVersions := range allAppVersions {
		for _, version := range appVersions.Versions {
			if !version.MarkedToRemoval {
				continue
			}
			pruned := pruneVersionFromRegistry(version)
			if !pruned {
				continue
			}

			pruneVersionFromStorage(appVersions.AppName, version)
		}
	}

	return multi.ToError()
}

func selectAppVersions(versions appTypes.AppVersions, deployedVersions []int, historySize int) (versionsToRemove, versionsToPruneFromProvisioner []appTypes.AppVersionInfo) {
	var regularVersions, customTagVersions []appTypes.AppVersionInfo
	for _, v := range versions.Versions {
		if v.MarkedToRemoval {
			continue
		} else if !v.DeploySuccessful {
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

	for i, version := range regularVersions {
		// never consider lastSuccessfulversion to garbage collection
		if i == 0 || version.Version == versions.LastSuccessfulVersion || intIn(version.Version, deployedVersions) {
			continue
		}
		if i >= historySize {
			versionsToRemove = append(versionsToRemove, version)
		} else {
			versionsToPruneFromProvisioner = append(versionsToPruneFromProvisioner, version)
		}
	}

	versionsToPruneFromProvisioner = append(versionsToPruneFromProvisioner, customTagVersions...)
	return
}

func pruneVersionFromRegistry(version appTypes.AppVersionInfo) (pruned bool) {
	pruned = true
	if version.DeployImage != "" {
		pruned = pruneImageFromRegistry(version.DeployImage) && pruned
	}

	if version.BuildImage != "" {
		pruned = pruneImageFromRegistry(version.BuildImage) && pruned
	}

	return pruned
}

func pruneImageFromRegistry(image string) (pruned bool) {
	registryPruneTotal.Inc()

	if err := registry.RemoveImageIgnoreNotFound(image); err != nil {
		log.Errorf("[image gc] error removing old image from registry %q: %s. Image kept on list to retry later.",
			image, err.Error())
		registryPruneFailuresTotal.Inc()
		return false
	}

	return true
}

func pruneVersionFromProvisioner(a *app.App, version appTypes.AppVersionInfo) (pruned bool) {
	pruned = true
	if version.DeployImage != "" {
		pruned = pruneImageFromProvisioner(a, version.DeployImage) && pruned
	}
	if version.BuildImage != "" {
		pruned = pruneImageFromProvisioner(a, version.BuildImage) && pruned
	}
	return pruned
}

func pruneImageFromProvisioner(a *app.App, image string) (pruned bool) {
	provisionerPruneTotal.Inc()

	err := a.CleanImage(image)
	if err != nil {
		log.Errorf("[image gc] error removing old image from provisioner for app %q: %v. Image kept on list to retry later.",
			image, err.Error())
		provisionerPruneFailuresTotal.Inc()
		return false
	}

	return true
}

func pruneVersionFromStorage(appName string, version appTypes.AppVersionInfo) {
	storagePruneTotal.Inc()

	err := servicemanager.AppVersion.DeleteVersion(appName, version.Version)
	if err != nil {
		log.Errorf("[image gc] error removing old version from database %q: %s", version, err)
		storagePruneFailuresTotal.Inc()
	}
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

func intIn(n int, slice []int) bool {
	for _, sliceN := range slice {
		if sliceN == n {
			return true
		}
	}
	return false
}
