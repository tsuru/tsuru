package main

import (
	"time"
	"github.com/timeredbull/tsuru/collector"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	var collector collector.Collector

	db, _ := sql.Open("sqlite3", "/home/ubuntu/cloudfoundry/vcap/cloud_controller/db/cloudcontroller.sqlite3")
	defer db.Close()

	c := time.Tick(1 * time.Minute)
	for _ = range c {
		data, _ := collector.Collect()
		output := collector.Parse(data)
		collector.Update(db, output)
	}
}
