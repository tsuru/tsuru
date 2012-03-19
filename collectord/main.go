package main

import (
	"time"
	"github.com/timeredbull/tsuru/collector"
)

func main() {
	var col collector.Collector
	c := time.Tick(1 * time.Minute)
	for _ = range c {
		data, _ := col.Collect()
		output := col.Parse(data)
		col.Update(output)
	}
}
