// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/heal"
	"net/http"
)

func getProvisioner() (string, error) {
	provisioner, err := config.GetString("provisioner")
	if provisioner == "" {
		provisioner = "juju"
	}
	return provisioner, err
}

// healers returns a json with all healers registered and yours endpoints.
func healers(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	p, _ := getProvisioner()
	h := map[string]string{}
	for healer := range heal.All(p) {
		h[healer] = fmt.Sprintf("/healers/%s", healer)
	}
	return json.NewEncoder(w).Encode(h)
}

func healer(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	p, _ := getProvisioner()
	healer, _ := heal.Get(p, r.URL.Query().Get(":healer"))
	w.WriteHeader(http.StatusOK)
	return healer.Heal()
}
