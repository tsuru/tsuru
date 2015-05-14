// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"time"

	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
)

type containerHealer struct {
	provisioner         *dockerProvisioner
	maxUnresponsiveTime time.Duration
	done                chan bool
}

func (h *containerHealer) runContainerHealer() {
	for {
		h.runContainerHealerOnce()
		select {
		case <-h.done:
			return
		case <-time.After(30 * time.Second):
		}
	}
}

func (h *containerHealer) Shutdown() {
	h.done <- true
}

func (h *containerHealer) String() string {
	return "container healer"
}

func (h *containerHealer) healContainer(cont container, locker *appLocker) (container, error) {
	var buf bytes.Buffer
	moveErrors := make(chan error, 1)
	createdContainer := h.provisioner.moveOneContainer(cont, "", moveErrors, nil, &buf, locker)
	close(moveErrors)
	err := handleMoveErrors(moveErrors, &buf)
	if err != nil {
		err = fmt.Errorf("Error trying to heal containers %s: couldn't move container: %s - %s", cont.ID, err.Error(), buf.String())
	}
	return createdContainer, err
}

func (h *containerHealer) getCommand(cont container) (string, error) {
	data, err := getImageCustomData(cont.Image)
	if err != nil {
		return "", err
	}
	if command, ok := data.Processes[cont.ProcessName]; ok {
		return command, nil
	}
	return "", fmt.Errorf("command %q not found in the image metadata", cont.ProcessName)
}

func (h *containerHealer) isProcessRunning(cont container) (bool, error) {
	command, err := h.getCommand(cont)
	if err != nil {
		return false, err
	}
	topResult, err := h.provisioner.getCluster().TopContainer(cont.ID, "")
	if err != nil {
		return false, err
	}
	for _, psLine := range topResult.Processes {
		process := psLine[len(psLine)-1]
		if process == command {
			return true, nil
		}
	}
	return false, nil
}

func (h *containerHealer) healContainerIfNeeded(cont container) error {
	if cont.LastSuccessStatusUpdate.IsZero() {
		return nil
	}
	isRunning, err := h.isProcessRunning(cont)
	if err != nil {
		log.Errorf("Containers healing: couldn't verify running processes in container %s: %s", cont.ID, err.Error())
	}
	if isRunning {
		cont.setStatus(h.provisioner, provision.StatusStarted.String())
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
	locker := &appLocker{}
	locked := locker.lock(cont.AppName)
	if !locked {
		return fmt.Errorf("Containers healing: unable to heal %s couldn't lock app %s", cont.ID, cont.AppName)
	}
	defer locker.unlock(cont.AppName)
	// Sanity check, now we have a lock, let's find out if the container still exists
	_, err = h.provisioner.getContainer(cont.ID)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return fmt.Errorf("Containers healing: unable to heal %s couldn't verify it still exists.", cont.ID)
	}
	log.Errorf("Initiating healing process for container %s, unresponsive since %s.", cont.ID, cont.LastSuccessStatusUpdate)
	evt, err := newHealingEvent(cont)
	if err != nil {
		return fmt.Errorf("Error trying to insert container healing event, healing aborted: %s", err.Error())
	}
	newCont, healErr := h.healContainer(cont, locker)
	if healErr != nil {
		healErr = fmt.Errorf("Error healing container %s: %s", cont.ID, healErr.Error())
	}
	err = evt.update(newCont, healErr)
	if err != nil {
		log.Errorf("Error trying to update containers healing event: %s", err.Error())
	}
	return healErr
}

func (h *containerHealer) runContainerHealerOnce() {
	containers, err := h.provisioner.listUnresponsiveContainers(h.maxUnresponsiveTime)
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
