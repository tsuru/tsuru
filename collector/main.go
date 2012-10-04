package main

import (
	"flag"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/api/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"labix.org/v2/mgo/bson"
	stdlog "log"
	"log/syslog"
	"time"
)

func getApps() []app.App {
	query := bson.M{
		"$or": []bson.M{
			bson.M{
				"units.agentstate": bson.M{"$ne": "started"},
			},
			bson.M{
				"units.machineagentstate": bson.M{"$ne": "running"},
			},
			bson.M{
				"units.instancestate": bson.M{"$ne": "running"},
			},
		},
	}
	var apps []app.App
	err := db.Session.Apps().Find(query).All(&apps)
	if err != nil {
		log.Panicf("Failed to get apps in the database: %s.", err)
	}
	return apps
}

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
		err        error
	)
	log.Target, err = syslog.NewLogger(syslog.LOG_INFO, stdlog.LstdFlags)
	if err != nil {
		panic(err)
	}
	flag.StringVar(&configFile, "config", "/etc/tsuru/tsuru.conf", "tsuru config file")
	flag.BoolVar(&dry, "dry", false, "dry-run: does not start the agent (for testing purposes)")
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
		ticker := time.Tick(time.Minute)
		jujuCollect(ticker)
	}
}
