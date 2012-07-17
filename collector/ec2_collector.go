package main

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	tec2 "github.com/timeredbull/tsuru/ec2"
	"github.com/timeredbull/tsuru/log"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goamz/ec2"
)

type Ec2Collector struct{}

func (ec *Ec2Collector) Collect() ([]ec2.Instance, error) {
	log.Print("Collecting ec2 instances state...")
	var srvInsts []service.ServiceInstance
	db.Session.ServiceInstances().Find(bson.M{"state": "creating"}).All(&srvInsts)
	instIds := make([]string, len(srvInsts))
	for i, inst := range srvInsts {
		instIds[i] = inst.Instance
	}
	if len(instIds) == 0 {
		log.Print("no service instances found for collect. Skipping...")
		return []ec2.Instance{}, nil
	}
	instResp, err := tec2.EC2.Instances(instIds, nil)
	if err != nil {
		return nil, err
	}
	var insts []ec2.Instance
	for _, rsvts := range instResp.Reservations {
		insts = append(insts, rsvts.Instances...)
	}
	return insts, nil
}

func (ec *Ec2Collector) Update(insts []ec2.Instance) error {
	log.Print("Updating service instance's state and attributes")
	var srvInsts []service.ServiceInstance
	q := bson.M{"instance": bson.M{"$in": instancesIds(insts)}}
	db.Session.ServiceInstances().Find(q).All(&srvInsts)
	for _, inst := range insts {
		for _, srvInst := range srvInsts {
			if srvInst.Instance == inst.InstanceId {
				msg := fmt.Sprintf("Updating instance %s with host %s, state %s and private host %s", inst.InstanceId, inst.DNSName, inst.State.Name, inst.PrivateDNSName)
				log.Print(msg)
				srvInst.State = inst.State.Name
				srvInst.Host = inst.DNSName
				srvInst.PrivateHost = inst.PrivateDNSName
				q = bson.M{"_id": srvInst.Name, "service_name": srvInst.ServiceName}
				db.Session.ServiceInstances().Update(q, srvInst)
			}
		}
	}
	return nil
}

func instancesIds(insts []ec2.Instance) []string {
	instIds := make([]string, len(insts))
	for i, inst := range insts {
		instIds[i] = inst.InstanceId
	}
	return instIds
}
