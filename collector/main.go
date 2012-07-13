package main

import (
	"flag"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/ec2"
	"github.com/timeredbull/tsuru/log"
	stdlog "log"
	"log/syslog"
	"sync"
	"time"
)

func ec2Collect() {
	var ec2Collector Ec2Collector
	ticker := time.Tick(time.Minute)
	_, err := ec2.Conn()
	if err != nil {
		log.Panicf("Got error while connecting with ec2: %s", err.Error())
	}
	for _ = range ticker {
		instances, err := ec2Collector.Collect()
		if err != nil {
			log.Printf("Error while collecting ec2 instances: %s.\nWill try again soon...", err.Error())
		} else {
			err = ec2Collector.Update(instances)
			if err != nil {
				log.Print("Error while updating database with collected data. Will try again soon...")
			}
		}
	}
}

func jujuCollect() {
	var collector Collector
	ticker := time.Tick(time.Minute)
	for _ = range ticker {
		data, _ := collector.Collect()
		output := collector.Parse(data)
		collector.Update(output)
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
	juju := flag.Bool("juju", true, "run juju collector")
	ec2 := flag.Bool("ec2", true, "run ec2 collector")
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
		var wg sync.WaitGroup
		if *ec2 {
			wg.Add(1)
			go func() {
				ec2Collect()
				wg.Done()
			}()
		}
		if *juju {
			wg.Add(1)
			go func() {
				jujuCollect()
				wg.Done()
			}()
		}
		wg.Wait()
	}
}
