package main

import (
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	tEC2 "github.com/timeredbull/tsuru/ec2"
	"github.com/timeredbull/tsuru/log"
	"launchpad.net/goamz/ec2"
)

func CollectEc2() ([]ec2.Instance, error) {
	log.Print("Collecting ec2 instances state...")
	var srvInsts []service.ServiceInstance
	db.Session.ServiceInstances().Find(nil).All(&srvInsts)
	instIds := make([]string, len(srvInsts))
	for i, inst := range srvInsts {
		instIds[i] = inst.Instance
	}
	if len(instIds) == 0 {
		log.Print("no service instances found for collect. Skipping...")
		return []ec2.Instance{}, nil
	}
	instResp, err := tEC2.EC2.Instances(instIds, nil)
	if err != nil {
		return nil, err
	}
	var insts []ec2.Instance
	for _, rsvts := range instResp.Reservations {
		insts = append(insts, rsvts.Instances...)
	}
	return insts, nil
}
