// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
)

type BindSyncer struct {
	Interval  time.Duration
	AppLister func() ([]bind.App, error)

	started  bool
	shutdown chan struct{}
	done     chan struct{}
}

// Start starts the sync process on a different goroutine
func (b *BindSyncer) Start() error {
	if b.started {
		return errors.New("syncer already started")
	}
	if b.AppLister == nil {
		return errors.New("must set app lister function")
	}
	if b.Interval == 0 {
		b.Interval = 5 * time.Minute
	}
	b.shutdown = make(chan struct{}, 1)
	b.done = make(chan struct{})
	b.started = true
	fmt.Printf("[bind-syncer] starting. Running every %s.\n", b.Interval)
	go func(d time.Duration) {
		for {
			select {
			case <-time.After(d):
				log.Debug("[bind-syncer] starting run")
				apps, err := b.AppLister()
				if err != nil {
					log.Errorf("[bind-syncer] error listing apps: %v. Aborting sync.", err)
					break
				}
				for _, a := range apps {
					err = b.sync(a)
					if err != nil {
						log.Errorf("[bind-syncer] error syncing app %q: %v", a.GetName(), err)
					}
				}
				log.Debugf("[bind-syncer] finished running. Synced %d apps.", len(apps))
				d = b.Interval
			case <-b.shutdown:
				b.done <- struct{}{}
				return
			}
		}
	}(time.Millisecond * 100)
	return nil
}

// Shutdown shutdowns BindSyncer waiting for the current sync
// to complete
func (b *BindSyncer) Shutdown(ctx context.Context) error {
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

func (b *BindSyncer) sync(a bind.App) (err error) {
	evt, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		InternalKind: "bindsyncer",
		Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permission.CtxApp, a.GetName())),
	})
	if err != nil {
		if _, ok := err.(event.ErrEventLocked); ok {
			log.Debugf("[bind-syncer] skipping sync for app %q: event locked", a.GetName())
			return nil
		}
		return errors.Wrap(err, "error trying to insert bind sync event, aborted")
	}
	defer func() { evt.Done(err) }()
	log.Debugf("[bind-syncer] starting sync for app %q", a.GetName())
	appUnits, err := a.GetUnits()
	if err != nil {
		return errors.Wrap(err, "error getting units from app")
	}
	units := make([]Unit, len(appUnits))
	for i := range appUnits {
		units[i] = Unit{ID: appUnits[i].GetID(), IP: appUnits[i].GetIp()}
	}
	instances, err := GetServiceInstancesBoundToApp(a.GetName())
	if err != nil {
		return errors.WithMessage(err, "error retrieving service instances bound to app")
	}
	for _, instance := range instances {
		boundUnits := make(map[Unit]struct{})
		for _, u := range instance.UnitsData {
			boundUnits[u] = struct{}{}
		}
		for _, u := range units {
			if _, ok := boundUnits[u]; ok {
				delete(boundUnits, u)
			} else {
				log.Debugf("[bind-syncer] binding unit %q from app %q from %s:%s\n", u.ID, a.GetName(), instance.ServiceName, instance.Name)
				err = instance.BindUnit(a, u)
				if err != nil {
					log.Errorf("[bind-syncer] failed to bind unit %q: %v", u.ID, err)
				}
			}
		}
		for u := range boundUnits {
			log.Debugf("[bind-syncer] unbinding unit %q from app %q from %s:%s\n", u.ID, a.GetName(), instance.ServiceName, instance.Name)
			err = instance.UnbindUnit(a, u)
			if err != nil {
				log.Errorf("[bind-syncer] failed to unbind unit %q: %v", u.ID, err)
			}
		}
	}
	log.Debugf("[bind-syncer] finished sync for app %q", a.GetName())
	return nil
}
