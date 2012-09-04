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
		log.Panicf("Failed to get apps in the database: %s.", err)
	}
	return apps
}

func jujuCollect(ticker <-chan time.Time, multitenant bool) {
	var (
		env string
		err error
		f   func()
	)
	if multitenant {
		f = func() {
			for _, app := range getApps() {
				data, _ := collect(app.JujuEnv)
				output := parse(data)
				update(output)
			}
		}
	} else {
		if env, err = config.GetString("juju:default-env"); err != nil {
			log.Panic("You must configure juju default-env in tsuru.conf.")
		}
		f = func() {
			data, _ := collect(env)
			output := parse(data)
			update(output)
		}
	}
	for _ = range ticker {
		f()
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
	multitenant, err := config.GetBool("multi-tenant")
	if err != nil {
		log.Panic(err)
	}

	if !dry {
		ticker := time.Tick(time.Minute)
		jujuCollect(ticker, multitenant)
	}
}
