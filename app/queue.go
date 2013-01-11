// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/service"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
)

const (
	// queue actions
	regenerateApprc         = "regenerate-apprc"
	startApp                = "start-app"
	RegenerateApprcAndStart = "regenerate-apprc-start-app"
	bindService             = "bind-service"

	// name of the queue for internal messages.
	queueName = "tsuru-app"

	// Name of the queue for external messages.
	QueueName = "tsuru-app-public"
)

func ensureAppIsStarted(msg *queue.Message) (App, error) {
	a := App{Name: msg.Args[0]}
	err := a.Get()
	if err != nil {
		return a, fmt.Errorf("Error handling %q: app %q does not exist.", msg.Action, a.Name)
	}
	units := getUnits(&a, msg.Args[1:])
	if !a.Available() || !units.Started() {
		format := "Error handling %q for the app %q:"
		switch a.State {
		case "error":
			format += " the app is in %q state."
			msg.Delete()
		case "down":
			format += " the app is %s."
			msg.Delete()
		default:
			format += ` The status of the app and all units should be "started" (the app is %q).`
			msg.Release(5e9)
		}
		return a, fmt.Errorf(format, msg.Action, a.Name, a.State)
	}
	return a, nil
}

func bindUnit(msg *queue.Message) error {
	a := App{Name: msg.Args[0]}
	err := a.Get()
	if err != nil {
		return fmt.Errorf("Error handling %q: app %q does not exist.", msg.Action, a.Name)
	}
	unit := getUnits(&a, msg.Args[1:])[0]
	var instances []service.ServiceInstance
	err = db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{msg.Args[0]}}}).All(&instances)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		_, err = instance.BindUnit(&unit)
		if err != nil {
			log.Printf("Error binding the unit %s with the service instance %s.", unit.Name, instance.Name)
		}
	}
	return nil
}

func handle(msg *queue.Message) {
	switch msg.Action {
	case RegenerateApprcAndStart:
		fallthrough
	case regenerateApprc:
		if len(msg.Args) < 1 {
			log.Printf("Error handling %q: this action requires at least 1 argument.", msg.Action)
			return
		}
		app, err := ensureAppIsStarted(msg)
		if err != nil {
			log.Print(err)
			return
		}
		msg.Delete()
		app.SerializeEnvVars()
		fallthrough
	case startApp:
		if msg.Action == regenerateApprc {
			break
		}
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
			return
		}
		msg.Delete()
	case bindService:
		err := bindUnit(msg)
		if err != nil {
			log.Print(err)
			return
		}
		msg.Delete()
	default:
		log.Printf("Error handling %q: invalid action.", msg.Action)
		msg.Delete()
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

var handler = &queue.Handler{F: handle, Queues: []string{queueName, QueueName}}

func enqueue(msgs ...queue.Message) {
	for _, msg := range msgs {
		copy := msg
		copy.Put(queueName, 0)
	}
	handler.Start()
}
