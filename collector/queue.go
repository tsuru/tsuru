// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"io/ioutil"
)

func handleMessages() {
	for {
		if message, err := queue.Get(5e9); err == nil {
			go handle(message)
		} else {
			log.Printf("Failed to receive message: %s. Trying again...", err)
			continue
		}
	}
}

func ensureAppIsStarted(msg *queue.Message) (app.App, error) {
	a := app.App{Name: msg.Args[0]}
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
	case app.RegenerateApprc:
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
	case app.StartApp:
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

type UnitList []app.Unit

func (l UnitList) Started() bool {
	for _, unit := range l {
		if unit.State != string(provision.StatusStarted) {
			return false
		}
	}
	return true
}

func getUnits(a *app.App, names []string) UnitList {
	var units []app.Unit
	if len(names) > 0 {
		units = make([]app.Unit, len(names))
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
	return UnitList(units)
}
