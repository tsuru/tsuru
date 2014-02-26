// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import "net/http"

func healthcheck(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("WORKING"))
}
