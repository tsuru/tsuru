// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"bytes"
	"context"
	"sync/atomic"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

type ContainerHealer struct {
	provisioner         DockerProvisioner
	maxUnresponsiveTime time.Duration
	done                chan bool
	locker              AppLocker
}

type ContainerHealerArgs struct {
	Provisioner         DockerProvisioner
	MaxUnresponsiveTime time.Duration
	Done                chan bool
	Locker              AppLocker
}

func NewContainerHealer(args ContainerHealerArgs) *ContainerHealer {
	return &ContainerHealer{
		provisioner:         args.Provisioner,
		maxUnresponsiveTime: args.MaxUnresponsiveTime,
		done:                args.Done,
		locker:              args.Locker,
	}
}

func (h *ContainerHealer) RunContainerHealer() {
	for {
		h.runContainerHealerOnce()
		select {
		case <-h.done:
			return
		case <-time.After(30 * time.Second):
		}
	}
}

func (h *ContainerHealer) Shutdown(ctx context.Context) error {
	h.done <- true
	return nil
}

func (h *ContainerHealer) String() string {
	return "container healer"
}

func (h *ContainerHealer) healContainer(cont container.Container) (container.Container, error) {
	var buf bytes.Buffer
	moveErrors := make(chan error, 1)
	createdContainer := h.provisioner.MoveOneContainer(cont, "", moveErrors, nil, &buf, h.locker)
	close(moveErrors)
	err := h.provisioner.HandleMoveErrors(moveErrors, &buf)
	if err != nil {
		err = errors.Wrapf(err, "Error trying to heal containers %s: couldn't move container. buffer: %s", cont.ID, buf.String())
	}
	return createdContainer, err
}

func (h *ContainerHealer) isAsExpected(cont container.Container) (bool, error) {
	container, err := h.provisioner.Cluster().InspectContainer(cont.ID)
	if err != nil {
		return false, err
	}
	if container.State.Dead || container.State.RemovalInProgress {
		return false, nil
	}
	isRunning := container.State.Running || container.State.Restarting
	if cont.ExpectedStatus() == provision.StatusStopped {
		return !isRunning, nil
	}
	return isRunning, nil
}

func (h *ContainerHealer) healContainerIfNeeded(cont container.Container) error {
	if cont.LastSuccessStatusUpdate.IsZero() {
		if !cont.MongoID.Time().Before(time.Now().Add(-h.maxUnresponsiveTime)) {
			return nil
		}
	}
	isAsExpected, err := h.isAsExpected(cont)
	if err != nil {
		log.Errorf("Containers healing: couldn't verify running processes in container %q: %s", cont.ID, err)
	}
	if isAsExpected {
		cont.SetStatus(h.provisioner.ClusterClient(), cont.ExpectedStatus(), true)
		return nil
	}
	locked := h.locker.Lock(cont.AppName)
	if !locked {
		return errors.Errorf("Containers healing: unable to heal %q couldn't lock app %s", cont.ID, cont.AppName)
	}
	defer h.locker.Unlock(cont.AppName)
	// Sanity check, now we have a lock, let's find out if the container still exists
	_, err = h.provisioner.GetContainer(cont.ID)
	if err != nil {
		if _, isNotFound := err.(*provision.UnitNotFoundError); isNotFound {
			return nil
		}
		return errors.Wrapf(err, "Containers healing: unable to heal %q couldn't verify it still exists", cont.ID)
	}
	a, err := app.GetByName(cont.AppName)
	if err != nil {
		return errors.Wrapf(err, "Containers healing: unable to heal %q couldn't get app %q", cont.ID, cont.AppName)
	}
	log.Errorf("Initiating healing process for container %q, unresponsive since %s.", cont.ID, cont.LastSuccessStatusUpdate)
	evt, err := event.NewInternal(&event.Opts{
		Target: event.Target{Type: event.TargetTypeContainer, Value: cont.ID},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: event.TargetTypeApp, Value: cont.AppName}},
		},
		InternalKind: "healer",
		CustomData:   cont,
		Allowed: event.Allowed(permission.PermAppReadEvents, append(permission.Contexts(permTypes.CtxTeam, a.Teams),
			permission.Context(permTypes.CtxApp, a.Name),
			permission.Context(permTypes.CtxPool, a.Pool),
		)...),
	})
	if err != nil {
		return errors.Wrap(err, "Error trying to insert container healing event, healing aborted")
	}
	newCont, healErr := h.healContainer(cont)
	if healErr != nil {
		healErr = errors.Errorf("Error healing container %q: %s", cont.ID, healErr.Error())
	}
	if newCont.ID != "" {
		evt.ExtraTargets = append(evt.ExtraTargets, event.ExtraTarget{Target: event.Target{Type: event.TargetTypeContainer, Value: newCont.ID}})
	}
	err = evt.DoneCustomData(healErr, newCont)
	if err != nil {
		log.Errorf("Error trying to update containers healing event: %s", err)
	}
	return healErr
}

func (h *ContainerHealer) runContainerHealerOnce() {
	containers, err := listUnresponsiveContainers(h.provisioner, h.maxUnresponsiveTime)
	if err != nil {
		log.Errorf("Containers Healing: couldn't list unresponsive containers: %s", err)
	}
	for _, cont := range containers {
		err := h.healContainerIfNeeded(cont)
		if err != nil {
			log.Errorf("Containers Healing: couldn't heal container: %s", err)
		}
	}
}

var localSkip uint64

func listUnresponsiveContainers(p DockerProvisioner, maxUnresponsiveTime time.Duration) ([]container.Container, error) {
	now := time.Now().UTC()
	conts, err := p.ListContainers(bson.M{
		"id":                      bson.M{"$ne": ""},
		"appname":                 bson.M{"$ne": ""},
		"lastsuccessstatusupdate": bson.M{"$lt": now.Add(-maxUnresponsiveTime)},
		"$or": []bson.M{
			{"hostport": bson.M{"$ne": ""}},
			{"processname": bson.M{"$ne": ""}},
		},
		"status": bson.M{"$nin": []string{
			provision.StatusBuilding.String(),
			provision.StatusAsleep.String(),
		}},
	})
	if err != nil {
		return nil, err
	}
	if len(conts) > 0 {
		pivot := atomic.AddUint64(&localSkip, 1) % uint64(len(conts))
		// Rotate the queried slice on pivot index to avoid the same node to always
		// be selected.
		conts = append(conts[pivot:], conts[:pivot]...)
	}
	return conts, nil
}
