// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collector

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	stdlog "log"
	"time"
)

func collect(ticker <-chan time.Time) {
	for _ = range ticker {
		log.Debug("Collecting status from provisioner")
		units, err := app.Provisioner.CollectStatus()
		if err != nil {
			log.Errorf("Failed to collect status within the provisioner: %s.", err)
			continue
		}
		update(units)
		log.Debug("Collecting status from provisioner finished")
	}
}

func fatal(err error) {
	stdlog.Fatal(err)
}

// Run is the function that starts the collector. The dryMode parameter
// indicates whether the collector should loop forever or not.
//
// It assumes the configuration has already been defined (from a config file or
// memory).
func Run(dryMode bool) {
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
	if !dryMode {
		provisioner, err := config.GetString("provisioner")
		if err != nil {
			fmt.Println("Warning: configuration didn't declare a provisioner, using default provisioner.")
			provisioner = "juju"
		}
		app.Provisioner, err = provision.Get(provisioner)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Using %q provisioner.\n\n", provisioner)

		timer, err := config.GetInt("collector:ticker-time")
		if err != nil {
			timer = 60
		}
		ticker := time.Tick(time.Duration(timer) * time.Second)
		fmt.Println("tsuru collector agent started...")
		collect(ticker)
	}
}
