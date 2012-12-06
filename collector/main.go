// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	stdlog "log"
	"log/syslog"
	"time"
)

func jujuCollect(ticker <-chan time.Time) {
	for _ = range ticker {
		data, _ := collect()
		output := parse(data)
		update(output)
	}
}

func main() {
	var (
		configFile string
		dry        bool
	)
	logger, err := syslog.NewLogger(syslog.LOG_INFO, stdlog.LstdFlags)
	if err != nil {
		panic(err)
	}
	log.SetLogger(logger)
	flag.StringVar(&configFile, "config", "/etc/tsuru/tsuru.conf", "tsuru config file")
	flag.BoolVar(&dry, "dry", false, "dry-run: does not start the agent neither the queue (for testing purposes)")
	flag.Parse()
	err = config.ReadConfigFile(configFile)
	if err != nil {
		log.Panic(err)
	}
	connString, err := config.GetString("database:url")
	if err != nil {
		log.Panic(err)
	}
	dbName, err := config.GetString("database:name")
	if err != nil {
		log.Panic(err)
	}
	db.Session, err = db.Open(connString, dbName)
	if err != nil {
		log.Panic(err)
	}
	defer db.Session.Close()

	if !dry {
		handler := MessageHandler{}
		handler.start()
		defer handler.stop()
		ticker := time.Tick(time.Minute)
		jujuCollect(ticker)
	}
}
