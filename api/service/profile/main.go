package main

import (
	"flag"
	"fmt"
	. "github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"log"
	"os"
	"runtime/pprof"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var memprofile = flag.String("memprofile", "", "write memory profile to file")

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru")
	if err != nil {
		panic(err)
	}
	defer db.Session.Close()

	sType := &ServiceType{Name: "Mysql", Charm: "mysql"}
	sType.Create()
	var s Service
	var name string
	for i := 0; i < 1000; i++ {
		name = fmt.Sprintf("myService%d", i)
		s = Service{ServiceTypeId: sType.Id, Name: name}
		s.Create()
	}
	s = Service{}
	s.All()

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}
}
