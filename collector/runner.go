// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collector

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	stdlog "log"
	"log/syslog"
	"os"
	"time"
)

func collect(ticker <-chan time.Time) {
	for _ = range ticker {
		log.Print("Collecting status from provisioner")
		units, err := app.Provisioner.CollectStatus()
		if err != nil {
			log.Printf("Failed to collect status within the provisioner: %s.", err)
			continue
		}
		update(units)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	log.Fatal(err)
}

func Run(flags map[string]interface{}) {
	logger, err := syslog.NewLogger(syslog.LOG_INFO, stdlog.LstdFlags)
	if err != nil {
		stdlog.Fatal(err)
	}
	log.SetLogger(logger)
	configFile, ok := flags["config"].(string)
	if !ok {
		configFile = "/etc/tsuru/tsuru.conf"
	}
	dry, ok := flags["dry"].(bool)
	if !ok {
		dry = false
	}
	err = config.ReadAndWatchConfigFile(configFile)
	if err != nil {
		fatal(err)
	}
	connString, err := config.GetString("database:url")
	if err != nil {
		fatal(err)
	}
	dbName, err := config.GetString("database:name")
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Using the database %q from the server %q.\n\n", dbName, connString)
	if !dry {
		provisioner, err := config.GetString("provisioner")
		if err != nil {
			fmt.Printf("Warning: %q didn't declare a provisioner, using default provisioner.\n", configFile)
			provisioner = "juju"
		}
		app.Provisioner, err = provision.Get(provisioner)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Using %q provisioner.\n\n", provisioner)

		ticker := time.Tick(time.Minute)
		fmt.Println("tsuru collector agent started...")
		collect(ticker)
	}
}
