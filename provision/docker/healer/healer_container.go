// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"bytes"
	"fmt"
	"time"

	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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

func (h *ContainerHealer) Shutdown() {
	h.done <- true
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
		err = fmt.Errorf("Error trying to heal containers %s: couldn't move container: %s - %s", cont.ID, err.Error(), buf.String())
	}
	return createdContainer, err
}

func (h *ContainerHealer) isRunning(cont container.Container) (bool, error) {
	container, err := h.provisioner.Cluster().InspectContainer(cont.ID)
	if err != nil {
		return false, err
	}
	return container.State.Running || container.State.Restarting, nil
}

func (h *ContainerHealer) healContainerIfNeeded(cont container.Container) error {
	if cont.LastSuccessStatusUpdate.IsZero() {
		return nil
	}
	isRunning, err := h.isRunning(cont)
	if err != nil {
		log.Errorf("Containers healing: couldn't verify running processes in container %s: %s", cont.ID, err.Error())
	}
	if isRunning {
		cont.SetStatus(h.provisioner, provision.StatusStarted.String(), true)
		return nil
	}
	healingCounter, err := healingCountFor("container", cont.ID, consecutiveHealingsTimeframe)
	if err != nil {
		return fmt.Errorf("Containers healing: couldn't verify number of previous healings for %s: %s", cont.ID, err.Error())
	}
	if healingCounter > consecutiveHealingsLimitInTimeframe {
		return fmt.Errorf("Containers healing: number of healings for container %s in the last %d minutes exceeds limit of %d: %d",
			cont.ID, consecutiveHealingsTimeframe/time.Minute, consecutiveHealingsLimitInTimeframe, healingCounter)
	}
	locked := h.locker.Lock(cont.AppName)
	if !locked {
		return fmt.Errorf("Containers healing: unable to heal %s couldn't lock app %s", cont.ID, cont.AppName)
	}
	defer h.locker.Unlock(cont.AppName)
	// Sanity check, now we have a lock, let's find out if the container still exists
	_, err = h.provisioner.GetContainer(cont.ID)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return fmt.Errorf("Containers healing: unable to heal %s couldn't verify it still exists.", cont.ID)
	}
	log.Errorf("Initiating healing process for container %s, unresponsive since %s.", cont.ID, cont.LastSuccessStatusUpdate)
	evt, err := NewHealingEvent(cont)
	if err != nil {
		return fmt.Errorf("Error trying to insert container healing event, healing aborted: %s", err.Error())
	}
	newCont, healErr := h.healContainer(cont)
	if healErr != nil {
		healErr = fmt.Errorf("Error healing container %s: %s", cont.ID, healErr.Error())
	}
	err = evt.Update(newCont, healErr)
	if err != nil {
		log.Errorf("Error trying to update containers healing event: %s", err.Error())
	}
	return healErr
}

func (h *ContainerHealer) runContainerHealerOnce() {
	containers, err := listUnresponsiveContainers(h.provisioner, h.maxUnresponsiveTime)
	if err != nil {
		log.Errorf("Containers Healing: couldn't list unresponsive containers: %s", err.Error())
	}
	for _, cont := range containers {
		err := h.healContainerIfNeeded(cont)
		if err != nil {
			log.Errorf(err.Error())
		}
	}
}

func listUnresponsiveContainers(p DockerProvisioner, maxUnresponsiveTime time.Duration) ([]container.Container, error) {
	now := time.Now().UTC()
	return p.ListContainers(bson.M{
		"lastsuccessstatusupdate": bson.M{"$lt": now.Add(-maxUnresponsiveTime)},
		"hostport":                bson.M{"$ne": ""},
		"status":                  bson.M{"$ne": provision.StatusStopped.String()},
	})
}
