// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	// "github.com/globocom/config"
	//"github.com/globocom/tsuru/app"
	// "github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/errors"
	// "github.com/globocom/tsuru/provision"
	// "github.com/globocom/tsuru/quota"
	// "github.com/globocom/tsuru/repository"
	// "github.com/globocom/tsuru/service"
	// "github.com/globocom/tsuru/testing"
	// "io"
	// "io/ioutil"
	// "labix.org/v2/mgo/bson"
	// "launchpad.net/gocheck"
	"net/http"
	// "net/http/httptest"
	// "sort"
	// "strconv"
	// "strings"
	// "sync/atomic"
	// "time"
)

func deploysList(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	appName := r.URL.Query().Get(":app")
	u, err := t.User()
	if err != nil {
		return err
	}
	a, err := getApp(appName, u)
	if err != nil {
		return err
	}
	deploys, err := a.ListDeploys()
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
	}
	return json.NewEncoder(w).Encode(deploys)
}
