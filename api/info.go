// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
)

func info(w http.ResponseWriter, r *http.Request) error {
	data := map[string]interface{}{}
	data["version"] = Version
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
}
