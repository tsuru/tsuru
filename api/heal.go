// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/heal"
	"net/http"
)

// healers returns a json with all healers registered and yours endpoints.
func healers(w http.ResponseWriter, r *http.Request) error {
	h := map[string]string{}
	for healer := range heal.All() {
		h[healer] = fmt.Sprintf("/healers/%s", healer)
	}
	return json.NewEncoder(w).Encode(h)
}

func healer(w http.ResponseWriter, r *http.Request) error {
	healer, _ := heal.Get(r.URL.Query().Get(":healer"))
	w.WriteHeader(http.StatusOK)
	return healer.Heal()
}
