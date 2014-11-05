// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type metrics struct {
	DataPoints [][]float64
}

func hasMetricsEnabled(app *App) bool {
	_, ok := app.Env["GRAPHITE_HOST"]
	return ok
}

func (app *App) Cpu() (float64, error) {
	if hasMetricsEnabled(app) {
		host := app.Env["GRAPHITE_HOST"].Value
		url := fmt.Sprintf("%s/render/?target=keepLastValue(maxSeries(statsite.tsuru.%s.*.*.cpu_max))&from=-10min", host, app.Name)
		resp, err := http.Get(url)
		var data []metrics
		err = json.NewDecoder(resp.Body).Decode(&data)
		if err != nil {
			return 0, errors.New("metrics disabled")
		}
		defer resp.Body.Close()
		if len(data[0].DataPoints) > 0 {
			index := len(data[0].DataPoints) - 1
			return data[0].DataPoints[index][0], nil
		}
	}
	return 0, errors.New("metrics disabled")
}
