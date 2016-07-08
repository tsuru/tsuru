// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/event"
)

// title: event list
// path: /event
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func eventList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	filter := &event.Filter{}
	if target := r.URL.Query().Get("target"); target != "" {
		filter.Target = event.Target{Name: target}
	}
	if running, err := strconv.ParseBool(r.URL.Query().Get("running")); err == nil {
		filter.Running = &running
	}
	events, err := event.List(filter)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(events)
}
