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
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	autoScaleEnabled, _ := config.GetBool("autoscale")
	if autoScaleEnabled {
		go runAutoScale()
	}
}

// AutoScaleEvent represents an auto scale event with
// the scale metadata.
type AutoScaleEvent struct {
	ID              bson.ObjectId `bson:"_id"`
	AppName         string
	StartTime       time.Time
	EndTime         time.Time `bson:",omitempty"`
	AutoScaleConfig *AutoScaleConfig
	Type            string
	Successful      bool
	Error           string `bson:",omitempty"`
}

func NewAutoScaleEvent(a *App, scaleType string) (*AutoScaleEvent, error) {
	evt := AutoScaleEvent{
		ID:              bson.NewObjectId(),
		StartTime:       time.Now().UTC(),
		AutoScaleConfig: a.AutoScaleConfig,
		AppName:         a.Name,
		Type:            scaleType,
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return &evt, conn.AutoScale().Insert(evt)
}

func (evt *AutoScaleEvent) update(err error) error {
	if err != nil {
		evt.Error = err.Error()
	}
	evt.Successful = err == nil
	evt.EndTime = time.Now().UTC()
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.AutoScale().UpdateId(evt.ID, evt)
}

// Action represents an AutoScale action to increase or decrease the
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
	MinUnits uint
	MaxUnits uint
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
		if wait, err := shouldWait(app, app.AutoScaleConfig.Increase.Wait); err != nil {
			return err
		} else if wait {
			return nil
		}
		_, err := AcquireApplicationLock(app.Name, InternalAppName, "auto-scale")
		if err != nil {
			return err
		}
		defer ReleaseApplicationLock(app.Name)
		evt, err := NewAutoScaleEvent(app, "increase")
		if err != nil {
			return fmt.Errorf("Error trying to insert auto scale event, auto scale aborted: %s", err.Error())
		}
		currentUnits := uint(len(app.Units()))
		inc := app.AutoScaleConfig.Increase.Units
		if currentUnits+inc > app.AutoScaleConfig.MaxUnits {
			inc = app.AutoScaleConfig.MaxUnits - currentUnits
		}
		addUnitsErr := app.AddUnits(inc, nil)
		err = evt.update(addUnitsErr)
		if err != nil {
			log.Errorf("Error trying to update auto scale event: %s", err.Error())
		}
		return addUnitsErr
	}
	decreaseMetric, _ := app.Metric(app.AutoScaleConfig.Decrease.metric())
	value, _ = app.AutoScaleConfig.Decrease.value()
	if decreaseMetric < value {
		if wait, err := shouldWait(app, app.AutoScaleConfig.Decrease.Wait); err != nil {
			return err
		} else if wait {
			return nil
		}
		_, err := AcquireApplicationLock(app.Name, InternalAppName, "auto-scale")
		if err != nil {
			return err
		}
		defer ReleaseApplicationLock(app.Name)
		evt, err := NewAutoScaleEvent(app, "decrease")
		if err != nil {
			return fmt.Errorf("Error trying to insert auto scale event, auto scale aborted: %s", err.Error())
		}
		currentUnits := uint(len(app.Units()))
		dec := app.AutoScaleConfig.Decrease.Units
		if currentUnits-dec < app.AutoScaleConfig.MinUnits {
			dec = currentUnits - app.AutoScaleConfig.MinUnits
		}
		removeUnitsErr := app.RemoveUnits(dec)
		err = evt.update(removeUnitsErr)
		if err != nil {
			log.Errorf("Error trying to update auto scale event: %s", err.Error())
		}
		return removeUnitsErr
	}
	return nil
}

func shouldWait(app *App, waitPeriod time.Duration) (bool, error) {
	now := time.Now().UTC()
	lastEvent, err := lastScaleEvent(app.Name)
	if err != nil && err != mgo.ErrNotFound {
		return false, err
	}
	if err != mgo.ErrNotFound && lastEvent.EndTime.IsZero() {
		return true, nil
	}
	diff := now.Sub(lastEvent.EndTime)
	if diff > waitPeriod {
		return false, nil
	}
	return true, nil
}

func lastScaleEvent(appName string) (AutoScaleEvent, error) {
	var event AutoScaleEvent
	conn, err := db.Conn()
	if err != nil {
		return event, err
	}
	defer conn.Close()
	err = conn.AutoScale().Find(bson.M{"appname": appName}).Sort("-starttime").One(&event)
	return event, err
}

func ListAutoScaleHistory(appName string) ([]AutoScaleEvent, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var history []AutoScaleEvent
	q := bson.M{}
	if appName != "" {
		q["appname"] = appName
	}
	err = conn.AutoScale().Find(q).Sort("-_id").Limit(200).All(&history)
	if err != nil {
		return nil, err
	}
	return history, nil
}

func AutoScaleEnable(app *App) error {
	if app.AutoScaleConfig == nil {
		app.AutoScaleConfig = &AutoScaleConfig{}
	}
	app.AutoScaleConfig.Enabled = true
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"autoscaleconfig": app.AutoScaleConfig}},
	)
}

func AutoScaleDisable(app *App) error {
	if app.AutoScaleConfig == nil {
		app.AutoScaleConfig = &AutoScaleConfig{}
	}
	app.AutoScaleConfig.Enabled = false
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"autoscaleconfig": app.AutoScaleConfig}},
	)
}

func SetAutoScaleConfig(app *App, config *AutoScaleConfig) error {
	app.AutoScaleConfig = config
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"autoscaleconfig": app.AutoScaleConfig}},
	)
}
