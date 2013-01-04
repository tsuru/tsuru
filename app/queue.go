// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"io/ioutil"
	"sync/atomic"
)

const (
	stopped int32 = iota
	running
	stopping
)

type messageHandler struct {
	state int32
}

// start starts the handler. It's safe to call start multiple times.
func (h *messageHandler) start() {
	if atomic.CompareAndSwapInt32(&h.state, stopped, running) {
		go h.handleMessages()
	}
}

// stop stops the handler. You can start it again by calling start.
func (h *messageHandler) stop() {
	atomic.StoreInt32(&h.state, stopping)
}

func (h *messageHandler) handleMessages() {
	for {
		if message, err := queue.Get(1e9); err == nil {
			go handle(message)
		} else if atomic.LoadInt32(&h.state) == running {
			log.Printf("Failed to receive message: %s. Trying again...", err)
			continue
		} else {
			break
		}
	}
	atomic.StoreInt32(&h.state, stopped)
}

var handler = &messageHandler{}

func ensureAppIsStarted(msg *queue.Message) (App, error) {
	a := App{Name: msg.Args[0]}
	err := a.Get()
	if err != nil {
		return a, fmt.Errorf("Error handling %q: app %q does not exist.", msg.Action, a.Name)
	}
	units := getUnits(&a, msg.Args[1:])
	if a.State != "started" || !units.Started() {
		format := "Error handling %q for the app %q:"
		switch a.State {
		case "error":
			format += " the app is in %q state."
			queue.Delete(msg)
		case "down":
			format += " the app is %s."
			queue.Delete(msg)
		default:
			format += ` The status of the app and all units should be "started" (the app is %q).`
		}
		return a, fmt.Errorf(format, msg.Action, a.Name, a.State)
	}
	return a, nil
}

func handle(msg *queue.Message) {
	switch msg.Action {
	case RegenerateApprc:
		if len(msg.Args) < 1 {
			log.Printf("Error handling %q: this action requires at least 1 argument.", msg.Action)
			return
		}
		app, err := ensureAppIsStarted(msg)
		if err != nil {
			log.Print(err)
			return
		}
		app.SerializeEnvVars()
	case StartApp:
		if len(msg.Args) < 1 {
			log.Printf("Error handling %q: this action requires at least 1 argument.", msg.Action)
		}
		app, err := ensureAppIsStarted(msg)
		if err != nil {
			log.Print(err)
			return
		}
		err = app.Restart(ioutil.Discard)
		if err != nil {
			log.Printf("Error handling %q. App failed to start:\n%s.", msg.Action, err)
		}
	default:
		log.Printf("Error handling %q: invalid action.", msg.Action)
	}
}

type unitList []Unit

func (l unitList) Started() bool {
	for _, unit := range l {
		if unit.State != string(provision.StatusStarted) {
			return false
		}
	}
	return true
}

func getUnits(a *App, names []string) unitList {
	var units []Unit
	if len(names) > 0 {
		units = make([]Unit, len(names))
		i := 0
		for _, unitName := range names {
			for _, appUnit := range a.Units {
				if appUnit.Name == unitName {
					units[i] = appUnit
					i++
					break
				}
			}
		}
	}
	return unitList(units)
}

func enqueue(msgs ...queue.Message) {
	for _, msg := range msgs {
		copy := msg
		queue.Put(&copy)
	}
	handler.start()
}
