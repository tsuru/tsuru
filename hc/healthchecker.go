// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hc provides a basic type for generic checks. With this packages,
// components can provide their own HealthChecker and register then for later
// use.
package hc

import "reflect"

// HealthCheckOK is the status returned when the healthcheck works.
const HealthCheckOK = "WORKING"

var checkers []HealthChecker

// HealthChecker represents a checker, that will be used to validate the
// current status of some component or service.
type HealthChecker interface {
	HealthCheck() error
}

// Result represents a result of a processed healthcheck call. It will contain
// the name of the healthchecker and the status returned in the HelthCheck()
// call.
type Result struct {
	Name   string
	Status string
}

// AddChecker adds a new checker to the internal list of checkers. Checkers
// added to this list can then be checked using the Check function.
func AddChecker(checker HealthChecker) {
	checkers = append(checkers, checker)
}

// Check check the status of all registered checkers and return a list of
// results.
func Check() []Result {
	results := make([]Result, len(checkers))
	for i, checker := range checkers {
		name := reflect.TypeOf(checker).Name()
		typeof := reflect.TypeOf(checker)
		if typeof.Kind() == reflect.Ptr {
			typeof = reflect.ValueOf(checker).Elem().Type()
			name = typeof.Name()
		}
		status := HealthCheckOK
		if err := checker.HealthCheck(); err != nil {
			status = "fail - " + err.Error()
		}
		results[i] = Result{Name: name, Status: status}
	}
	return results
}
