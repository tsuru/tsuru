// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	stdlog "log"
	"log/syslog"
	"os"
	"time"
)

func jujuCollect(ticker <-chan time.Time) {
	for _ = range ticker {
		units, err := app.Provisioner.CollectStatus()
		if err != nil {
			log.Printf("Failed to collect status withing the provisioner: %s.", err)
		}
		update(units)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	log.Fatal(err)
}

func main() {
	var (
		configFile string
		dry        bool
	)
	logger, err := syslog.NewLogger(syslog.LOG_INFO, stdlog.LstdFlags)
	if err != nil {
		stdlog.Fatal(err)
	}
	log.SetLogger(logger)
	flag.StringVar(&configFile, "config", "/etc/tsuru/tsuru.conf", "tsuru config file")
	flag.BoolVar(&dry, "dry", false, "dry-run: does not start the agent neither the queue (for testing purposes)")
	flag.Parse()
	err = config.ReadConfigFile(configFile)
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
	db.Session, err = db.Open(connString, dbName)
	if err != nil {
		fatal(err)
	}
	defer db.Session.Close()
	fmt.Printf("Connected to MongoDB server at %s.\n", connString)
	fmt.Printf("Using the database %q.\n\n", dbName)

	if !dry {
		handler := MessageHandler{}
		err = handler.start()
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Queue server listening at %s.\n", handler.server.Addr())
		defer handler.stop()
		ticker := time.Tick(time.Minute)
		fmt.Println("tsuru collector agent started...")
		jujuCollect(ticker)
	}
}
