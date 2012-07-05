package main

import (
	"flag"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"time"
)

func main() {
	var (
		collector    Collector
		ec2Collector Ec2Collector
		err          error
	)

	dry := flag.Bool("dry", false, "dry-run: does not start the agent (for testing purposes)")
	flag.Parse()
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru")
	if err != nil {
		log.Panic(err.Error())
	}
	defer db.Session.Close()

	if !*dry {
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
