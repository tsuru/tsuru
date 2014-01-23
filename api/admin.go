// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/bmizerany/pat"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"net"
	"net/http"
)

var m = pat.New()

func RegisterAdminHandler(path string, method string, h http.Handler) {
	if method == "GET" {
		m.Get(path, h)
	}
	if method == "POST" {
		m.Post(path, h)
	}
	if method == "PUT" {
		m.Put(path, h)
	}
	if method == "DELETE" {
		m.Del(path, h)
	}
}

// RunAdminServer starts tsuru administrative api
func RunAdminServer(dry bool) {
	log.Init()
	connString, err := config.GetString("database:url")
	if err != nil {
		connString = db.DefaultDatabaseURL
	}
	dbName, err := config.GetString("database:name")
	if err != nil {
		dbName = db.DefaultDatabaseName
	}
	fmt.Printf("Using the database %q from the server %q.\n\n", dbName, connString)
	if !dry {
		provisioner, err := getProvisioner()
		if err != nil {
			fmt.Printf("Warning: configuration didn't declare a provisioner, using default provisioner.\n")
		}
		app.Provisioner, err = provision.Get(provisioner)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Using %q provisioner.\n\n", provisioner)

		listen, err := config.GetString("admin-listen")
		if err != nil {
			fatal(err)
		}
		listener, err := net.Listen("tcp", listen)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("tsuru HTTP server listening at %s...\n", listen)
		http.Handle("/", m)
		fatal(http.Serve(listener, nil))
	}
}
