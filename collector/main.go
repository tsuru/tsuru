// +build ignore

package main

import (
	"."
	. "github.com/timeredbull/tsuru/database"
	"launchpad.net/mgo"
	"time"
)

func main() {
	var collector collector.Collector

	session, err := mgo.Dial("localhost:27017")
	if err != nil {
		panic(err)
	}
	Db = session.DB("tsuru")
	defer session.Close()

	c := time.Tick(1 * time.Minute)
	for _ = range c {
		data, _ := collector.Collect()
		output := collector.Parse(data)
		collector.Update(output)
	}
}
