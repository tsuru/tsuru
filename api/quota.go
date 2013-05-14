// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/quota"
	"net/http"
)

func quotaByUser(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	result := map[string]interface{}{}
	user := r.URL.Query().Get(":user")
	items, available, err := quota.Items(user)
	if err != nil {
		return err
	}
	result["items"] = items
	result["available"] = available
	return json.NewEncoder(w).Encode(result)
}
