// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package metrics provides interfaces that need to be satisfied in order to
// implement a new metric backend on tsuru.
package metrics

var dbs = make(map[string]TimeSeriesDatabase)

// TimeSeriesDatabase is the basic interface of this package. It provides methods for
// time series databases.
type TimeSeriesDatabase interface {
	Summarize(key, interval, function string) (Series, error)
}

type Series []Data

type Data struct {
	Timestamp float64
	Value     float64
}

// Register registers a new time series database.
func Register(name string, db TimeSeriesDatabase) {
	dbs[name] = db
}
