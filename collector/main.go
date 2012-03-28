// +build ignore

package main

import (
	"."
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"time"
)

func main() {
	var collector collector.Collector

	db, _ := sql.Open("sqlite3", "../api/webserverd/tsuru.db")
	defer db.Close()

	c := time.Tick(1 * time.Minute)
	for _ = range c {
		data, _ := collector.Collect()
		output := collector.Parse(data)
		collector.Update(db, output)
	}
}
