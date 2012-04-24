package main

import (
	"flag"
	"fmt"
	"launchpad.net/mgo"
	"log"
	"os"
	"runtime/pprof"
	. "github.com/timeredbull/tsuru/api/service"
	. "github.com/timeredbull/tsuru/database"
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

	session, err := mgo.Dial("localhost:27017")
	if err != nil {
		panic(err)
	}
	Db = session.DB("tsuru")
	defer session.Close()

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

	// c := Db.C("services")
	// defer c.DropCollection()
}
