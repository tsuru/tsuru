// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package metricsinterfaces that need to be satisfied in order to
// implement a new metric backend on tsuru.
package metrics

// TimeSeriesDatabase is the basic interface of this package. It provides methods for
// time series databases.
type TimeSeriesDatabase interface {
	Summarize(key, interval, funtion string) []interface{}
}
