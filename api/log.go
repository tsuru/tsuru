// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/auth"
	"net/http"
)

func logRemove(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	return app.LogRemove(nil)
}
