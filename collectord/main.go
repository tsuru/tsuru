package main

import (
	"time"
	"github.com/timeredbull/tsuru/collector"
)

func main() {
	var xpto collector.Collector
	c := time.Tick(1 * time.Minute)
	for now := range c {
		data, _ := xpto.Collect()
		output := xpto.Parse(data)
		xpto.Update(output)
	}
}
