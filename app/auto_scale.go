// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2/bson"
)

// autoScaleEvent represents an auto scale event with
// the scale metadata.
type autoScaleEvent struct {
	ID              bson.ObjectId `bson:"_id"`
	AppName         string
	StartTime       time.Time
	EndTime         time.Time `bson:",omitempty"`
	AutoScaleConfig *AutoScaleConfig
	Type            string
	Successful      bool
	Error           string `bson:",omitempty"`
}

func autoScaleCollection() (*storage.Collection, error) {
	name, _ := config.GetString("autoscale:events_collection")
	if name == "" {
		name = "autoscale_events"
	}
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err.Error())
		return nil, err
	}
	return conn.Collection(name), nil
}

func newAutoScaleEvent(a *App, scaleType string) (*autoScaleEvent, error) {
	evt := autoScaleEvent{
		ID:              bson.NewObjectId(),
		StartTime:       time.Now().UTC(),
		AutoScaleConfig: a.AutoScaleConfig,
		AppName:         a.Name,
		Type:            scaleType,
	}
	coll, err := autoScaleCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	return &evt, coll.Insert(evt)
}

func (evt *autoScaleEvent) update(err error) error {
	if err != nil {
		evt.Error = err.Error()
	}
	evt.Successful = err == nil
	evt.EndTime = time.Now().UTC()
	coll, err := autoScaleCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.UpdateId(evt.ID, evt)
}

// Action represents an AutoScale action to increase or decreate the
// number of the units.
type Action struct {
	Wait       time.Duration
	Expression string
	Units      uint
}

func NewAction(expression string, units uint, wait time.Duration) (*Action, error) {
	if expressionIsValid(expression) {
		return &Action{Wait: wait, Expression: expression, Units: units}, nil
	}
	return nil, errors.New("Expression is not valid.")
}

var expressionRegex = regexp.MustCompile("{(.*)} ([><=]) ([0-9]+)")

func expressionIsValid(expression string) bool {
	return expressionRegex.MatchString(expression)
}

func (action *Action) metric() string {
	return expressionRegex.FindStringSubmatch(action.Expression)[1]
}

func (action *Action) operator() string {
	return expressionRegex.FindStringSubmatch(action.Expression)[2]
}

func (action *Action) value() (float64, error) {
	return strconv.ParseFloat(expressionRegex.FindStringSubmatch(action.Expression)[3], 64)
}

// AutoScaleConfig represents the App configuration for the auto scale.
type AutoScaleConfig struct {
	Increase Action
	Decrease Action
	MinUnits int
	MaxUnits int
	Enabled  bool
}

func autoScalableApps() ([]App, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var apps []App
	err = conn.Apps().Find(bson.M{"autoscaleconfig.enabled": true}).All(&apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func runAutoScaleOnce() {
	apps, err := autoScalableApps()
	if err != nil {
		log.Error(err.Error())
	}
	for _, app := range apps {
		err := scaleApplicationIfNeeded(&app)
		if err != nil {
			log.Error(err.Error())
		}
	}
}

func runAutoScale() {
	for {
		runAutoScaleOnce()
		time.Sleep(30 * time.Second)
	}
}

func scaleApplicationIfNeeded(app *App) error {
	if app.AutoScaleConfig == nil {
		return errors.New("AutoScale is not configured.")
	}
	increaseMetric, _ := app.Metric(app.AutoScaleConfig.Increase.metric())
	value, _ := app.AutoScaleConfig.Increase.value()
	if increaseMetric > value {
		_, err := AcquireApplicationLock(app.Name, InternalAppName, "auto-scale")
		if err != nil {
			return err
		}
		defer ReleaseApplicationLock(app.Name)
		evt, err := newAutoScaleEvent(app, "increase")
		if err != nil {
			return fmt.Errorf("Error trying to insert auto scale event, auto scale aborted: %s", err.Error())
		}
		autoScaleErr := app.AddUnits(app.AutoScaleConfig.Increase.Units, nil)
		err = evt.update(autoScaleErr)
		if err != nil {
			log.Errorf("Error trying to update auto scale event: %s", err.Error())
		}
		return autoScaleErr
	}
	decreaseMetric, _ := app.Metric(app.AutoScaleConfig.Decrease.metric())
	value, _ = app.AutoScaleConfig.Decrease.value()
	if decreaseMetric < value {
		_, err := AcquireApplicationLock(app.Name, InternalAppName, "auto-scale")
		if err != nil {
			return err
		}
		defer ReleaseApplicationLock(app.Name)
		evt, err := newAutoScaleEvent(app, "decrease")
		if err != nil {
			return fmt.Errorf("Error trying to insert auto scale event, auto scale aborted: %s", err.Error())
		}
		autoScaleErr := app.RemoveUnits(app.AutoScaleConfig.Decrease.Units)
		err = evt.update(autoScaleErr)
		if err != nil {
			log.Errorf("Error trying to update auto scale event: %s", err.Error())
		}
		return autoScaleErr
	}
	return nil
}
