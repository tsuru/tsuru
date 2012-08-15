package main

import (
	"flag"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
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
		log.Panicf("Failed to get apps in the database: %s.", err.Error())
	}
	return apps
}

func jujuCollect(ticker <-chan time.Time) {
	var apps []app.App
	for _ = range ticker {
		apps = getApps()
		for _, app := range apps {
			data, _ := collect(&app)
			output := parse(data)
			update(output)
		}
	}
}

func main() {
	var err error
	log.Target, err = syslog.NewLogger(syslog.LOG_INFO, stdlog.LstdFlags)
	if err != nil {
		panic(err)
	}
	configFile := flag.String("config", "/etc/tsuru/tsuru.conf", "tsuru config file")
	dry := flag.Bool("dry", false, "dry-run: does not start the agent (for testing purposes)")
	flag.Parse()
	err = config.ReadConfigFile(*configFile)
	if err != nil {
		log.Panic(err.Error())
	}
	connString, err := config.GetString("database:url")
	if err != nil {
		panic(err)
	}
	dbName, err := config.GetString("database:name")
	if err != nil {
		panic(err)
	}
	db.Session, err = db.Open(connString, dbName)
	if err != nil {
		log.Panic(err.Error())
	}
	defer db.Session.Close()

	if !*dry {
		ticker := time.Tick(time.Minute)
		jujuCollect(ticker)
	}
}
