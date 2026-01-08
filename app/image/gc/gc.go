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
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/registry"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/provision"
	"go.opentelemetry.io/otel"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	imageGCRunInterval = 5 * time.Minute
	promNamespace      = "tsuru"
	promSubsystem      = "gc"
)

var (
	gcExecutionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "executions_total",
		Help:      "The number of times that gc had runned by phase",
	}, []string{"phase"})

	executionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "execution_duration_seconds",
		Help:      "How long during the GC process",
		Buckets:   prometheus.ExponentialBuckets(0.1, 2.7, 10),
	}, []string{"phase"})

	versionsMarkedToRemovalTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "versions_marked_to_removal_total",
		Help:      "The number of versions of applications that was marked to removal",
	})

	// registry metrics
	registryPruneTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "registry_prune_total",
		Help:      "The number of executions of prune against the registry",
	})

	registryPruneFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "registry_prune_failures_total",
		Help:      "The number of failures to prune unused images from registry",
	})

	registryPruneDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "registry_prune_duration_seconds",
		Help:      "How long during single prune to registry",
		Buckets:   prometheus.ExponentialBuckets(0.005, 4, 10),
	})

	// database metrics
	storagePruneTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "storage_prune_total",
		Help:      "The number of executions of prune against the storage",
	})

	storagePruneFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "storage_prune_failures_total",
		Help:      "The number of failures to prune unused images from storage",
	})

	storagePruneDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "storage_prune_duration_seconds",
		Help:      "How long during single prune to storage",
		Buckets:   prometheus.ExponentialBuckets(0.005, 4, 10),
	})
)

func init() {
	event.SetThrottling(event.ThrottlingSpec{
		TargetType: eventTypes.TargetTypeGC,
		KindName:   "gc",
		Time:       imageGCRunInterval,
		Max:        1,
		AllTargets: true,
		WaitFinish: true,
	})
}

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
		runPeriodicGC()

		select {
		case <-g.stopCh:
			return
		case <-time.After(imageGCRunInterval):
		}
	}
}

func runPeriodicGC() (err error) {
	ctx := context.Background()
	eventExpireAt := time.Now().Add(180 * 24 * time.Hour) // 6 months
	evt, err := event.NewInternal(ctx, &event.Opts{
		Target:       eventTypes.Target{Type: eventTypes.TargetTypeGC, Value: "global"},
		InternalKind: "gc",
		Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxGlobal, "")),
		ExpireAt:     &eventExpireAt,
	})
	defer func() {
		if err != nil {
			log.Errorf("[image gc] %v", err)
		}
		if evt == nil {
			return
		}
		if err == nil {
			evt.Abort(ctx)
		} else {
			evt.Done(ctx, err)
		}
	}()

	if err != nil {
		_, isThrottled := err.(event.ErrThrottled)
		_, isLocked := err.(event.ErrEventLocked)
		if isThrottled || isLocked {
			gcExecutionsTotal.WithLabelValues("suspended").Inc()
			err = nil
			return
		}

		err = errors.Wrap(err, "could not create event")
		return
	}

	multi := tsuruErrors.NewMultiError()
	err = markOldImages(ctx)
	if err != nil {
		multi.Add(errors.Wrap(err, "errors running GC mark"))
	}

	dryRun, err := config.GetBool("docker:gc:dry-run")
	if err != nil {
		multi.Add(errors.Wrap(err, "fetch config error"))
	}
	if dryRun {
		err = multi.ToError()
		return
	}

	err = sweepOldImages()
	if err != nil {
		multi.Add(errors.Wrap(err, "errors running GC sweep"))
	}

	err = multi.ToError()
	return
}

func markOldImages(ctx context.Context) error {
	eventExpireAt := time.Now().Add(180 * 24 * time.Hour) // 6 months

	tracer := otel.Tracer("tsuru/app/image/gc")
	ctx, span := tracer.Start(ctx, "GC markOldImages")
	defer span.End()

	gcExecutionsTotal.WithLabelValues("mark").Inc()
	timer := prometheus.NewTimer(executionDuration.WithLabelValues("mark"))
	defer timer.ObserveDuration()

	log.Debugf("[image gc] starting gc process to select old images")
	defer log.Debugf("[image gc] finished gc process to select old images")
	allAppVersions, err := servicemanager.AppVersion.AllAppVersions(ctx)
	if err != nil {
		return err
	}
	historySize := image.ImageHistorySize()
	multi := tsuruErrors.NewMultiError()
	for _, appVersions := range allAppVersions {
		if len(appVersions.Versions) == 0 {
			continue
		}

		log.Debugf("[image gc] processing %d versions for app %q", len(appVersions.Versions), appVersions.AppName)
		a, err := app.GetByName(ctx, appVersions.AppName)
		if err != nil && err != appTypes.ErrAppNotFound {
			multi.Add(err)
			continue
		}
		if a == nil {
			log.Debugf("[image gc] app %q not found, mark everything to removal", appVersions.AppName)
			err = servicemanager.AppVersion.MarkToRemoval(ctx, appVersions.AppName, &appTypes.AppVersionWriteOptions{
				PreviousUpdatedHash: appVersions.UpdatedHash,
			})

			if err != nil && err != appTypes.ErrTransactionCancelledByChange {
				multi.Add(err)
			}
			continue
		}

		requireExclusiveLock, err := markOldImagesForAppVersion(ctx, a, appVersions, historySize, false)
		if err != nil {
			multi.Add(err)
			continue
		}
		if !requireExclusiveLock {
			continue
		}

		evt, err := event.NewInternal(ctx, &event.Opts{
			Target:       eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: appVersions.AppName},
			InternalKind: "version gc",
			Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, appVersions.AppName)),
			ExpireAt:     &eventExpireAt,
		})

		if err != nil {
			if _, ok := err.(event.ErrEventLocked); ok {
				continue
			}
			multi.Add(errors.Wrapf(err, "unable to acquire lock of app: %q", appVersions.AppName))
			continue
		}

		_, err = markOldImagesForAppVersion(ctx, a, appVersions, historySize, true)
		if err != nil {
			multi.Add(err)
		}
		evt.Done(ctx, err)
	}
	return multi.ToError()
}

func markOldImagesForAppVersion(ctx context.Context, a *appTypes.App, appVersions appTypes.AppVersions, historySize int, exclusiveLockAcquired bool) (requireExclusiveLock bool, err error) {
	deployedVersions, err := app.DeployedVersions(ctx, a)
	causeErr := errors.Cause(err)
	if causeErr == app.ErrNoVersionProvisioner || causeErr == provision.ErrNoCluster {
		deployedVersions = []int{appVersions.LastSuccessfulVersion}
	} else if err != nil {
		return false, errors.Wrapf(err, "Could not get deployed versions of app: %s", appVersions.AppName)
	}

	selection := selectAppVersions(appVersions, deployedVersions, historySize)
	if len(selection.toRemove) == 0 && len(selection.unsuccessfulDeploys) == 0 {
		return false, nil
	}
	if !exclusiveLockAcquired {
		return true, nil
	}

	// we can not remove a running deployment version
	// to accomplish that, let's check the every EventID whether is running.
	if len(selection.unsuccessfulDeploys) > 0 {
		var toRemove []appTypes.AppVersionInfo
		toRemove, err = versionsSafeToRemove(ctx, selection.unsuccessfulDeploys)
		if err != nil {
			return false, errors.Wrapf(err, "Could not check events running of app: %s", appVersions.AppName)
		}

		selection.toRemove = append(selection.toRemove, toRemove...)
	}

	versionIDs := []int{}
	for _, version := range selection.toRemove {
		versionsMarkedToRemovalTotal.Inc()
		versionIDs = append(versionIDs, version.Version)
		log.Debugf("[image gc] marking version %d for removal from app %q - DeployImage: %s, BuildImage: %s", version.Version, a.Name, version.DeployImage, version.BuildImage)
	}

	if len(versionIDs) > 0 {
		log.Debugf("[image gc] marked %d versions from app %q to removal: %v", len(versionIDs), a.Name, versionIDs)
	}

	err = servicemanager.AppVersion.MarkVersionsToRemoval(ctx, a.Name, versionIDs, &appTypes.AppVersionWriteOptions{
		PreviousUpdatedHash: appVersions.UpdatedHash,
	})

	if err != nil && err != appTypes.ErrTransactionCancelledByChange {
		return false, errors.Wrapf(err, "Could not mark versions to removal of app: %s", appVersions.AppName)
	}
	return false, nil
}

// versionsSafeToRemove checks whether a version does have a related event running
func versionsSafeToRemove(ctx context.Context, appVersions []appTypes.AppVersionInfo) ([]appTypes.AppVersionInfo, error) {
	uniqueIds := []primitive.ObjectID{}
	mapEventID := map[string]appTypes.AppVersionInfo{}

	for _, v := range appVersions {
		if v.EventID == "" {
			continue
		}

		objID, err := primitive.ObjectIDFromHex(v.EventID)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not convert eventID to ObjectID: %s", v.EventID)
		}

		uniqueIds = append(uniqueIds, objID)
		mapEventID[v.EventID] = v
	}

	events, err := event.List(ctx, &event.Filter{
		Raw: mongoBSON.M{
			"uniqueid": mongoBSON.M{
				"$in": uniqueIds,
			},
		},
	})

	if err != nil {
		return nil, err
	}

	safeVersions := []appTypes.AppVersionInfo{}
	for _, event := range events {
		if event.Running || event.EndTime.IsZero() {
			continue
		}

		version, found := mapEventID[event.UniqueID.Hex()]
		if !found {
			continue
		}
		safeVersions = append(safeVersions, version)
	}

	return safeVersions, nil
}

func sweepOldImages() error {
	tracer := otel.Tracer("tsuru/app/image/gc")
	ctx, span := tracer.Start(context.Background(), "GC sweepOldImages")
	defer span.End()

	gcExecutionsTotal.WithLabelValues("sweep").Inc()
	timer := prometheus.NewTimer(executionDuration.WithLabelValues("sweep"))
	defer timer.ObserveDuration()

	log.Debugf("[image gc] starting gc process to sweep old images")
	defer log.Debugf("[image gc] finished gc process to sweep old images")

	allAppVersions, err := servicemanager.AppVersion.AllAppVersions(ctx)
	if err != nil {
		return err
	}

	multi := tsuruErrors.NewMultiError()
	versionsToRemove := map[string][]appTypes.AppVersionInfo{}
	versionsIDsToRemove := map[string][]int{}
	mapAppVersions := map[string]appTypes.AppVersions{}
	for _, appVersions := range allAppVersions {
		if appVersions.MarkedToRemoval {
			err = pruneAllVersionsByApp(ctx, appVersions)
			if err != nil {
				multi.Add(err)
			}
			continue
		}
		for _, version := range appVersions.Versions {
			if !version.MarkedToRemoval {
				continue
			}
			versionsToRemove[appVersions.AppName] = append(versionsToRemove[appVersions.AppName], version)
			versionsIDsToRemove[appVersions.AppName] = append(versionsIDsToRemove[appVersions.AppName], version.Version)
		}
		if len(versionsIDsToRemove[appVersions.AppName]) > 0 {
			mapAppVersions[appVersions.AppName] = appVersions
		}
	}

	for appName, versions := range versionsToRemove {
		if err == appTypes.ErrAppNotFound {
			// in the next mark process will be removed
			continue
		}
		if err != nil {
			multi.Add(err)
			continue
		}

		log.Debugf("[image gc] sweeping %d versions from app %q", len(versions), appName)

		versionsToRemove := []int{}
		for _, version := range versions {
			log.Debugf("[image gc] sweeping version %d from app %q - DeployImage: %s, BuildImage: %s", version.Version, appName, version.DeployImage, version.BuildImage)
			err = pruneVersionFromRegistry(ctx, version)
			if err != nil {
				multi.Add(err)
				continue
			}

			versionsToRemove = append(versionsToRemove, version.Version)
		}

		if len(versionsToRemove) > 0 {
			log.Debugf("[image gc] removing %d versions from storage for app %q: %v", len(versionsToRemove), appName, versionsToRemove)
			err = pruneVersionFromStorage(ctx, mapAppVersions[appName], versionsToRemove)
			if err != nil {
				multi.Add(err)
				continue
			}
			log.Debugf("[image gc] successfully removed %d versions from storage for app %q", len(versionsToRemove), appName)
		}
	}

	return multi.ToError()
}

type appVersionsSelection struct {
	toRemove            []appTypes.AppVersionInfo
	unsuccessfulDeploys []appTypes.AppVersionInfo
}

func selectAppVersions(versions appTypes.AppVersions, deployedVersions []int, historySize int) *appVersionsSelection {
	var regularVersions, customTagVersions []appTypes.AppVersionInfo
	selection := &appVersionsSelection{}
	for _, v := range versions.Versions {
		if v.MarkedToRemoval {
			continue
		} else if v.CustomBuildTag != "" {
			customTagVersions = append(customTagVersions, v)
		} else if !v.DeploySuccessful {
			// A point to remember: @wpjunior
			// All deploys are created with flag above as a false value
			// It means in the future will turned to true, to avoid a remotion of a running event please check whether v.EventID is running.
			selection.unsuccessfulDeploys = append(selection.unsuccessfulDeploys, v)
		} else {
			regularVersions = append(regularVersions, v)
		}
	}

	sort.Sort(priorizedAppVersions(selection.unsuccessfulDeploys))
	sort.Sort(priorizedAppVersions(regularVersions))
	sort.Sort(priorizedAppVersions(customTagVersions))

	for i, version := range regularVersions {
		// never consider lastSuccessfulversion to garbage collection
		if i == 0 || version.Version == versions.LastSuccessfulVersion || intIn(version.Version, deployedVersions) {
			continue
		}
		if i >= historySize {
			selection.toRemove = append(selection.toRemove, version)
		}
	}

	return selection
}

func pruneAllVersionsByApp(ctx context.Context, appVersions appTypes.AppVersions) error {
	multi := tsuruErrors.NewMultiError()

	err := registry.RemoveAppImages(ctx, appVersions.AppName)
	if err != nil {
		multi.Add(errors.Wrapf(err, "could not remove images from registry, app: %q", appVersions.AppName))
	}
	err = servicemanager.AppVersion.DeleteVersions(ctx, appVersions.AppName, &appTypes.AppVersionWriteOptions{
		PreviousUpdatedHash: appVersions.UpdatedHash,
	})
	if err != nil && err != appTypes.ErrTransactionCancelledByChange {
		multi.Add(errors.Wrapf(err, "could not remove versions from storage, app: %q", appVersions.AppName))
	}

	return multi.ToError()
}

func pruneVersionFromRegistry(ctx context.Context, version appTypes.AppVersionInfo) error {
	multi := tsuruErrors.NewMultiError()

	if version.DeployImage != "" {
		err := pruneImageFromRegistry(ctx, version.DeployImage)
		if err != nil {
			multi.Add(err)
		}
	}

	if version.BuildImage != "" {
		err := pruneImageFromRegistry(ctx, version.BuildImage)
		if err != nil {
			multi.Add(err)
		}
	}

	return multi.ToError()
}

func pruneImageFromRegistry(ctx context.Context, image string) error {
	registryPruneTotal.Inc()
	timer := prometheus.NewTimer(registryPruneDuration)
	defer timer.ObserveDuration()

	log.Debugf("[image gc] removing image from registry: %s", image)

	if err := registry.RemoveImageIgnoreNotFound(ctx, image); err != nil {
		err = errors.Wrapf(err, "error removing old image from registry %q. Image kept on list to retry later.", image)
		log.Errorf("[image gc] %s", err.Error())
		registryPruneFailuresTotal.Inc()
		return err
	}

	log.Debugf("[image gc] successfully removed image from registry: %s", image)
	return nil
}

func pruneVersionFromStorage(ctx context.Context, appVersions appTypes.AppVersions, versions []int) error {
	storagePruneTotal.Inc()
	timer := prometheus.NewTimer(storagePruneDuration)
	defer timer.ObserveDuration()

	err := servicemanager.AppVersion.DeleteVersionIDs(ctx, appVersions.AppName, versions, &appTypes.AppVersionWriteOptions{
		PreviousUpdatedHash: appVersions.UpdatedHash,
	})
	if err != nil && err != appTypes.ErrTransactionCancelledByChange {
		err = errors.Wrapf(err, "error removing old versions from database for app: %q", appVersions.AppName)
		log.Errorf("[image gc] %s", err.Error())
		storagePruneFailuresTotal.Inc()
	}
	return nil
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

func intIn(n int, slice []int) bool {
	for _, sliceN := range slice {
		if sliceN == n {
			return true
		}
	}
	return false
}
