// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"

	"github.com/tsuru/tsuru/metrics"
	_ "github.com/tsuru/tsuru/metrics/graphite"
)

func (app *App) Metric(kind string) (float64, error) {
	conf := map[string]string{}
	for _, env := range app.Env {
		conf[env.Name] = env.Value
	}
	db, err := metrics.Get(conf)
	if err != nil {
		return 0, errors.New("metrics disabled")
	}
	key := fmt.Sprintf("%s.*.*.%s", app.Name, kind)
	series, err := db.Summarize(key, "-10h", "max")
	if err != nil {
		return 0, err
	}
	return series[len(series)-1].Value, err
}
