// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app/bind"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

var (
	syncOperations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_service_sync_operations_total",
		Help: "The total number of sync operations.",
	}, []string{"op"})

	syncErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_service_sync_operations_errors_total",
		Help: "The total number of sync errors.",
	}, []string{"op"})

	syncDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_service_sync_last_duration",
		Help: "The duration of the last sync operation.",
	})
)

func init() {
	prometheus.MustRegister(syncOperations, syncErrors, syncDuration)
}

func InitializeSync(appLister func() ([]bind.App, error)) error {
	interval, _ := config.GetDuration("service:sync:interval")
	if interval <= 0 {
		interval = time.Minute * 5
	}
	syncer := &bindSyncer{
		interval:  interval,
		appLister: appLister,
	}
	err := syncer.start()
	if err != nil {
		return err
	}
	shutdown.Register(syncer)
	return nil
}

type bindSyncer struct {
	interval  time.Duration
	appLister func() ([]bind.App, error)

	started  bool
	shutdown chan struct{}
	done     chan struct{}
}

// start starts the sync process on a different goroutine
func (b *bindSyncer) start() error {
	if b.started {
		return errors.New("syncer already started")
	}
	if b.appLister == nil {
		return errors.New("must set app lister function")
	}
	if b.interval == 0 {
		b.interval = 5 * time.Minute
	}
	b.shutdown = make(chan struct{}, 1)
	b.done = make(chan struct{})
	b.started = true
	log.Debugf("[bind-syncer] starting. Running every %s.\n", b.interval)
	go func(d time.Duration) {
		for {
			select {
			case <-time.After(d):
				start := time.Now()
				log.Debug("[bind-syncer] starting run")
				apps, err := b.appLister()
				if err != nil {
					log.Errorf("[bind-syncer] error listing apps: %v. Aborting sync.", err)
					syncDuration.Set(time.Since(start).Seconds())
					break
				}
				for _, a := range apps {
					err = b.sync(a)
					if err != nil {
						log.Errorf("[bind-syncer] error syncing app %q: %v", a.GetName(), err)
					}
					if len(b.shutdown) > 0 {
						break
					}
				}
				log.Debugf("[bind-syncer] finished running. Synced %d apps.", len(apps))
				d = b.interval
				syncDuration.Set(time.Since(start).Seconds())
			case <-b.shutdown:
				b.done <- struct{}{}
				return
			}
		}
	}(time.Millisecond * 100)
	return nil
}

// Shutdown shutdowns bindSyncer waiting for the current sync
// to complete
func (b *bindSyncer) Shutdown(ctx context.Context) error {
	if !b.started {
		return nil
	}
	b.shutdown <- struct{}{}
	select {
	case <-b.done:
	case <-ctx.Done():
	}
	b.started = false
	return ctx.Err()
}

func (b *bindSyncer) sync(a bind.App) (err error) {
	binds := make(map[string][]string)
	unbinds := make(map[string][]string)
	evt, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		InternalKind: "bindsyncer",
		Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, a.GetName())),
	})
	if err != nil {
		if _, ok := err.(event.ErrEventLocked); ok {
			log.Debugf("[bind-syncer] skipping sync for app %q: event locked", a.GetName())
			return nil
		}
		return errors.Wrap(err, "error trying to insert bind sync event, aborted")
	}
	defer func() {
		if len(binds)+len(unbinds) > 0 || err != nil {
			evt.DoneCustomData(err, map[string]interface{}{
				"binds":   binds,
				"unbinds": unbinds,
			})
			return
		}
		evt.Logf("nothing to do for %q", a.GetName())
		evt.Abort()
	}()
	log.Debugf("[bind-syncer] starting sync for app %q", a.GetName())
	appUnits, err := a.GetUnits()
	if err != nil {
		return errors.Wrap(err, "error getting units from app")
	}
	units := make([]Unit, len(appUnits))
	for i := range appUnits {
		units[i] = Unit{AppName: a.GetName(), ID: appUnits[i].GetID(), IP: appUnits[i].GetIp()}
	}
	instances, err := GetServiceInstancesBoundToApp(a.GetName())
	if err != nil {
		return errors.WithMessage(err, "error retrieving service instances bound to app")
	}
	multiErr := tsuruErrors.NewMultiError()
	for _, instance := range instances {
		boundUnits := make(map[Unit]struct{})
		for _, u := range instance.BoundUnits {
			if u.AppName != a.GetName() {
				continue
			}
			boundUnits[u] = struct{}{}
		}
		for _, u := range units {
			if _, ok := boundUnits[u]; !ok {
				log.Debugf("[bind-syncer] binding unit %q from app %q from %s:%s\n", u.ID, a.GetName(), instance.ServiceName, instance.Name)
				err = instance.BindUnit(a, u)
				binds[instance.Name] = append(binds[instance.Name], u.GetID())
				if err != nil {
					err = errors.Wrapf(err, "failed to bind unit %q for %s(%s)", u.ID, instance.ServiceName, instance.Name)
					multiErr.Add(err)
					syncErrors.WithLabelValues("bind").Inc()
				}
				syncOperations.WithLabelValues("bind").Inc()
			}
			delete(boundUnits, u)
		}
		for u := range boundUnits {
			log.Debugf("[bind-syncer] unbinding unit %q from app %q from %s:%s\n", u.ID, a.GetName(), instance.ServiceName, instance.Name)
			err = instance.UnbindUnit(a, u)
			unbinds[instance.Name] = append(unbinds[instance.Name], u.GetID())
			if err != nil {
				err = errors.Wrapf(err, "failed to unbind unit %q for %s(%s)", u.ID, instance.ServiceName, instance.Name)
				multiErr.Add(err)
				syncErrors.WithLabelValues("unbind").Inc()
			}
			syncOperations.WithLabelValues("unbind").Inc()
		}
	}
	log.Debugf("[bind-syncer] finished sync for app %q", a.GetName())
	return multiErr.ToError()
}
