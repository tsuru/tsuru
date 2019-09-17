// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
)

// title: api info
// path: /info
// method: GET
// produce: application/json
// responses:
//   200: OK
func info(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	data := map[string]string{}
	data["version"] = Version
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
}
