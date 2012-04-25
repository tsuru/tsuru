// +build ignore

package main

import (
	"."
	"github.com/timeredbull/tsuru/db"
	"time"
)

func main() {
	var collector collector.Collector
	var err error

	db.Session, err = db.Open("127.0.0.1:27017", "tsuru")
	if err != nil {
		panic(err)
	}
	defer db.Session.Close()

	c := time.Tick(1 * time.Minute)
	for _ = range c {
		data, _ := collector.Collect()
		output := collector.Parse(data)
		collector.Update(output)
	}
}
