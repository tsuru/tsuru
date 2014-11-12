// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
)

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
}

func allApps() ([]App, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var apps []App
	err = conn.Apps().Find(nil).All(&apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func runAutoScaleOnce() {
	apps, err := allApps()
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
		return app.AddUnits(app.AutoScaleConfig.Increase.Units, nil)
	}
	decreaseMetric, _ := app.Metric(app.AutoScaleConfig.Decrease.metric())
	value, _ = app.AutoScaleConfig.Decrease.value()
	if decreaseMetric < value {
		_, err := AcquireApplicationLock(app.Name, InternalAppName, "auto-scale")
		if err != nil {
			return err
		}
		defer ReleaseApplicationLock(app.Name)
		return app.RemoveUnits(app.AutoScaleConfig.Decrease.Units)
	}
	return nil
}
