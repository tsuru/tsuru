// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/auth"
)

func deprecatedHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.WriteHeader(http.StatusGone)
	fmt.Fprintf(w, "This route has been deprecated")
	return nil
}
