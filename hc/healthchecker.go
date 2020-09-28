// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hc provides a basic type for generic checks. With this packages,
// components can provide their own HealthChecker and register then for later
// use.
package hc

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/set"
)

// HealthCheckOK is the status returned when the healthcheck works.
const HealthCheckOK = "WORKING"

var ErrDisabledComponent = errors.New("disabled component")

var checkers []healthChecker

type healthChecker struct {
	name  string
	check func(ctx context.Context) error
}

// Result represents a result of a processed healthcheck call. It will contain
// the name of the healthchecker and the status returned in the checker
// call.
type Result struct {
	Name     string
	Status   string
	Duration time.Duration
}

// AddChecker adds a new checker to the internal list of checkers. Checkers
// added to this list can then be checked using the Check function.
func AddChecker(name string, check func(ctx context.Context) error) {
	checker := healthChecker{name: name, check: check}
	checkers = append(checkers, checker)
}

// Check check the status of registered checkers matching names and return a
// list of results.
func Check(ctx context.Context, names ...string) []Result {
	results := make([]Result, 0, len(checkers))
	nameSet := set.FromSlice(names)
	isAll := nameSet.Includes("all")
	for _, checker := range checkers {
		if !isAll && !nameSet.Includes(checker.name) {
			continue
		}
		startTime := time.Now()
		if err := checker.check(ctx); err != nil && err != ErrDisabledComponent {
			results = append(results, Result{
				Name:     checker.name,
				Status:   "fail - " + err.Error(),
				Duration: time.Since(startTime),
			})
		} else if err == nil {
			results = append(results, Result{
				Name:     checker.name,
				Status:   HealthCheckOK,
				Duration: time.Since(startTime),
			})
		}
	}
	return results
}
