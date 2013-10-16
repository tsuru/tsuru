// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/quota"
	"net/http"
	"strconv"
)

func quotaByOwner(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	result := map[string]interface{}{}
	owner := r.URL.Query().Get(":owner")
	items, available, err := quota.Items(owner)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	result["items"] = items
	result["available"] = available
	return json.NewEncoder(w).Encode(result)
}

func changeQuota(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	owner := r.URL.Query().Get(":owner")
	q, err := strconv.Atoi(r.PostFormValue("quota"))
	if err != nil {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "invalid parameter"}
	}
	err = quota.Set(owner, uint(q))
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	w.Write([]byte("Quota changed sucessfully."))
	return nil
}
