// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/service"
	"labix.org/v2/mgo/bson"
	"sync"
)

const (
	// queue actions
	regenerateApprc = "regenerate-apprc"
	BindService     = "bind-service"

	queueName = "tsuru-app"
)

// ensureAppIsStarted make sure that the app and all units present in the given
// message are started.
func ensureAppIsStarted(msg *queue.Message) (App, error) {
	app, err := GetByName(msg.Args[0])
	if err != nil {
		log.Errorf("Error handling %q: app %q does not exist.", msg.Action, msg.Args[0])
		return App{}, fmt.Errorf("Error handling %q: app %q does not exist.", msg.Action, msg.Args[0])
	}
	units := getUnits(app, msg.Args[1:])
	if len(msg.Args) > 1 && len(units) == 0 {
		format := "Error handling %q for the app %q: unknown units in the message. Deleting it..."
		log.Errorf(format, msg.Action, app.Name)
		return *app, fmt.Errorf(format, msg.Action, app.Name)
	}
	if !app.Available() || !units.Started() {
		format := "Error handling %q for the app %q:"
		uState := units.State()
		if uState == "error" || uState == "down" {
			format += fmt.Sprintf(" units are in %q state.", uState)
			log.Errorf(format, msg.Action, app.Name)
		} else {
			msg.Fail()
			format += " all units must be started."
		}
		return *app, fmt.Errorf(format, msg.Action, app.Name)
	}
	return *app, nil
}

// bindUnit handles the bind-service message, binding a unit to all service
// instances bound to the app.
func bindUnit(msg *queue.Message) error {
	app, err := GetByName(msg.Args[0])
	if err != nil {
		log.Errorf("Error handling %q: app %q does not exist.", msg.Action, msg.Args[0])
		return fmt.Errorf("Error handling %q: app %q does not exist.", msg.Action, msg.Args[0])
	}
	conn, err := db.Conn()
	if err != nil {
		return fmt.Errorf("Error handling %q: %s", msg.Action, err)
	}
	defer conn.Close()
	units := getUnits(app, msg.Args[1:])
	if len(units) == 0 {
		return errors.New("Unknown unit in the message.")
	}
	unit := units[0]
	var instances []service.ServiceInstance
	q := bson.M{"apps": bson.M{"$in": []string{app.Name}}}
	err = conn.ServiceInstances().Find(q).All(&instances)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		_, err = instance.BindUnit(app, &unit)
		if err != nil {
			log.Errorf("Error binding the unit %s with the service instance %s: %s", unit.Name, instance.Name, err)
		}
	}
	return nil
}

// handle is the function called by the queue handler on each message.
func handle(msg *queue.Message) {
	switch msg.Action {
	case regenerateApprc:
		if len(msg.Args) < 1 {
			log.Errorf("Error handling %q: this action requires at least 1 argument.", msg.Action)
			return
		}
		app, err := ensureAppIsStarted(msg)
		if err != nil {
			return
		}
		err = app.SerializeEnvVars()
		if err != nil {
			log.Error(err.Error())
		}
	case BindService:
		err := bindUnit(msg)
		if err != nil {
			log.Error(err.Error())
			return
		}
	default:
		log.Errorf("Error handling %q: invalid action.", msg.Action)
	}
}

// unitList is a simple slice of units, with special methods to handle state.
type unitList []provision.Unit

// Started returns true if all units in the list is started.
func (l unitList) Started() bool {
	for _, unit := range l {
		if !unit.Available() {
			return false
		}
	}
	return true
}

// State returns a string if all units have the same status. Otherwise it
// returns an empty string.
func (l unitList) State() string {
	if len(l) == 0 {
		return ""
	}
	status := l[0].Status.String()
	for i := 1; i < len(l); i++ {
		if l[i].Status.String() != status {
			return ""
		}
	}
	return status
}

// getUnits builds a unitList from the given app and the names in the string
// slice.
func getUnits(app *App, names []string) unitList {
	var units []provision.Unit
	if len(names) > 0 {
		for _, unitName := range names {
			for _, appUnit := range app.Units() {
				if appUnit.Name == unitName {
					units = append(units, appUnit)
					break
				}
			}
		}
	}
	return unitList(units)
}

var (
	qfactory queue.QFactory
	_queue   queue.Q
	_handler queue.Handler
	o        sync.Once
)

func setQueue() {
	var err error
	qfactory, err = queue.Factory()
	if err != nil {
		log.Errorf("Failed to get the queue instance: %s", err)
	}
	_handler, err = qfactory.Handler(handle, queueName)
	if err != nil {
		log.Errorf("Failed to create the queue handler: %s", err)
	}
	_queue, err = qfactory.Get(queueName)
	if err != nil {
		log.Errorf("Failed to get the queue instance: %s", err)
	}
}

func handler() queue.Handler {
	o.Do(setQueue)
	return _handler
}

func aqueue() queue.Q {
	o.Do(setQueue)
	return _queue
}

// Enqueue puts the given message in the queue. The app queue is able to handle
// only messages defined in this package.
//
// Here is a functional example for this function:
//
//     msg := queue.Message{Action: app.regenerateApprc, Args: []string{"myapp"}}
//     app.Enqueue(msg)
func Enqueue(msgs ...queue.Message) {
	q := aqueue()
	for _, msg := range msgs {
		copy := msg
		q.Put(&copy, 0)
	}
	handler().Start()
}
