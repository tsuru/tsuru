package main

import (
	"flag"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/ec2"
	"github.com/timeredbull/tsuru/log"
	stdlog "log"
	"log/syslog"
	"time"
)

func main() {
	var (
		collector    Collector
		ec2Collector Ec2Collector
		err          error
	)
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
		_, err = ec2.Conn()
		if err != nil {
			log.Print("Got error while connecting with ec2:")
			log.Print(err.Error())
		}
		c := time.Tick(time.Minute)
		for _ = range c {
			data, _ := collector.Collect()
			output := collector.Parse(data)
			collector.Update(output)
			instances, err := ec2Collector.Collect()
			if err != nil {
				log.Print("Error while collecting ec2 instances. Will try again soon...")
			}
			err = ec2Collector.Update(instances)
			if err != nil {
				log.Print("Error while updating database with collected data. Will try again soon...")
			}
		}
	}
}
