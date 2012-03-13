package main

import (
	"time"
	"github.com/timeredbull/tsuru/collector"
)

func main() {
	var collector collector.Collector
	c := time.Tick(1 * time.Minute)
	for _ = range c {
		data, _ := collector.Collect()
		output := collector.Parse(data)
		collector.Update(output)
	}
}
