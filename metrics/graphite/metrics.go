// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package graphite

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/metrics"
)

func init() {
	metrics.Register("graphite", graphite{})
}

type graphiteData struct {
	DataPoints [][]float64
}

// graphite represents the Graphite time series database.
type graphite struct{}

func getHost() string {
	host, err := config.GetString("graphite:host")
	if err != nil {
		return ""
	}
	if !strings.Contains(host, "http") {
		host = fmt.Sprintf("http://%s", host)
	}
	return host
}

// Summarize summarizes the data into interval buckets of a certain size.
func (g graphite) Summarize(key, interval, function string) (metrics.Series, error) {
	url := fmt.Sprintf("%s/render/?target=keepLastValue(maxSeries(%s))&from=%s&format=json", getHost(), key, interval)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	var series metrics.Series
	var data []graphiteData
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New("metrics disabled")
	}
	defer resp.Body.Close()
	for _, dataPoint := range data[0].DataPoints {
		series = append(series, metrics.Data{
			Value:     dataPoint[0],
			Timestamp: dataPoint[1],
		})
	}
	return series, nil
}
