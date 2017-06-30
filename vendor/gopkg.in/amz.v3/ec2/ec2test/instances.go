//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011-2015 Canonical Ltd.
//
// This file contains all code handling AWS API around instances.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gopkg.in/amz.v3/ec2"
)

// Recognized AWS instance states.
var (
	Pending      = ec2.InstanceState{0, "pending"}
	Running      = ec2.InstanceState{16, "running"}
	ShuttingDown = ec2.InstanceState{32, "shutting-down"}
	Terminated   = ec2.InstanceState{16, "terminated"}
	Stopped      = ec2.InstanceState{16, "stopped"}
)

// Instance holds a simulated ec2 instance
type Instance struct {
	seq int
	// first is set to true until the instance has been marshaled
	// into a response at least once.
	first bool
	// UserData holds the data that was passed to the RunInstances request
	// when the instance was started.
	UserData            []byte
	imageId             string
	reservation         *reservation
	instType            string
	availZone           string
	state               ec2.InstanceState
	subnetId            string
	vpcId               string
	ifaces              []ec2.NetworkInterface
	blockDeviceMappings []ec2.InstanceBlockDeviceMapping
	sourceDestCheck     bool
	tags                []ec2.Tag
	rootDeviceType      string
	rootDeviceName      string
}

// SetInitialInstanceState sets the state that any new instances will be started in.
func (srv *Server) SetInitialInstanceState(state ec2.InstanceState) {
	srv.mu.Lock()
	srv.initialInstanceState = state
	srv.mu.Unlock()
}

// NewInstancesVPC creates n new VPC instances in srv with the given
// instance type, image ID, initial state, and security groups,
// belonging to the given vpcId and subnetId. If any group does not
// already exist, it will be created. NewInstancesVPC returns the ids
// of the new instances.
//
// If vpcId and subnetId are both empty, this call is equivalent to
// calling NewInstances.
func (srv *Server) NewInstancesVPC(vpcId, subnetId string, n int, instType string, imageId string, state ec2.InstanceState, groups []ec2.SecurityGroup) []string {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	rgroups := make([]*securityGroup, len(groups))
	for i, group := range groups {
		g := srv.group(group)
		if g == nil {
			fatalf(400, "InvalidGroup.NotFound", "no such group %v", g)
		}
		rgroups[i] = g
	}
	r := srv.newReservation(rgroups)

	ids := make([]string, n)
	for i := 0; i < n; i++ {
		inst := srv.newInstance(r, instType, imageId, defaultAvailZone, state)
		inst.vpcId = vpcId
		inst.subnetId = subnetId
		ids[i] = inst.id()
	}
	return ids
}

// NewInstances creates n new instances in srv with the given instance
// type, image ID, initial state, and security groups. If any group
// does not already exist, it will be created. NewInstances returns
// the ids of the new instances.
func (srv *Server) NewInstances(n int, instType string, imageId string, state ec2.InstanceState, groups []ec2.SecurityGroup) []string {
	return srv.NewInstancesVPC("", "", n, instType, imageId, state, groups)
}

// Instance returns the instance for the given instance id.
// It returns nil if there is no such instance.
func (srv *Server) Instance(id string) *Instance {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.instances[id]
}

/// Instance internals

func (inst *Instance) id() string {
	return fmt.Sprintf("i-%d", inst.seq)
}

func (inst *Instance) terminate() (d ec2.InstanceStateChange) {
	d.PreviousState = inst.state
	inst.state = ShuttingDown
	d.CurrentState = inst.state
	d.InstanceId = inst.id()
	return d
}

func (inst *Instance) ec2instance() ec2.Instance {
	id := inst.id()
	// The first time the instance is returned, its DNSName
	// and block device mappings will be empty. The client
	// should then refresh the instance.
	var dnsName string
	var blockDeviceMappings []ec2.InstanceBlockDeviceMapping
	if inst.first {
		inst.first = false
	} else {
		dnsName = fmt.Sprintf("%s.testing.invalid", id)
		blockDeviceMappings = inst.blockDeviceMappings
	}
	return ec2.Instance{
		InstanceId:          id,
		InstanceType:        inst.instType,
		ImageId:             inst.imageId,
		DNSName:             dnsName,
		PrivateDNSName:      fmt.Sprintf("%s.internal.invalid", id),
		IPAddress:           fmt.Sprintf("8.0.0.%d", inst.seq%256),
		PrivateIPAddress:    fmt.Sprintf("127.0.0.%d", inst.seq%256),
		State:               inst.state,
		AvailZone:           inst.availZone,
		VPCId:               inst.vpcId,
		SubnetId:            inst.subnetId,
		NetworkInterfaces:   inst.ifaces,
		BlockDeviceMappings: blockDeviceMappings,
		SourceDestCheck:     inst.sourceDestCheck,
		Tags:                inst.tags,
		RootDeviceType:      inst.rootDeviceType,
		RootDeviceName:      inst.rootDeviceName,
		// TODO the rest
	}
}

func (inst *Instance) matchAttr(attr, value string) (ok bool, err error) {
	if strings.HasPrefix(attr, "tag:") && len(attr) > 4 {
		filterTag := attr[4:]
		return matchTag(inst.tags, filterTag, value), nil
	}
	switch attr {
	case "architecture":
		return value == "i386", nil
	case "availability-zone":
		return value == inst.availZone, nil
	case "instance-id":
		return inst.id() == value, nil
	case "subnet-id":
		return inst.subnetId == value, nil
	case "vpc-id":
		return inst.vpcId == value, nil
	case "instance.group-id", "group-id":
		for _, g := range inst.reservation.groups {
			if g.id == value {
				return true, nil
			}
		}
		return false, nil
	case "instance.group-name", "group-name":
		for _, g := range inst.reservation.groups {
			if g.name == value {
				return true, nil
			}
		}
		return false, nil
	case "image-id":
		return value == inst.imageId, nil
	case "instance-state-code":
		code, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return code&0xff == inst.state.Code, nil
	case "instance-state-name":
		return value == inst.state.Name, nil
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

/// Server instance internals.

func (srv *Server) newInstance(r *reservation, instType string, imageId string, availZone string, state ec2.InstanceState) *Instance {
	inst := &Instance{
		seq:             srv.maxId.next(),
		first:           true,
		instType:        instType,
		imageId:         imageId,
		availZone:       availZone,
		state:           state,
		reservation:     r,
		sourceDestCheck: true,
	}
	id := inst.id()
	srv.instances[id] = inst
	r.instances[id] = inst

	if srv.createRootDisks {
		// create a root disk for the instance
		inst.rootDeviceType = "ebs"
		inst.rootDeviceName = "/dev/sda1"
		volume := srv.newVolume("magnetic", 8)
		volume.AvailZone = availZone
		volume.Status = "in-use"
		volumeAttachment := &volumeAttachment{}
		volumeAttachment.InstanceId = inst.id()
		volumeAttachment.Status = "attached"
		volumeAttachment.DeleteOnTermination = true
		volumeAttachment.Device = inst.rootDeviceName
		srv.volumeAttachments[volume.Id] = volumeAttachment
		inst.blockDeviceMappings = []ec2.InstanceBlockDeviceMapping{{
			DeviceName:          inst.rootDeviceName,
			VolumeId:            volume.Id,
			AttachTime:          time.Now().Format(time.RFC3339),
			Status:              volumeAttachment.Status,
			DeleteOnTermination: volumeAttachment.DeleteOnTermination,
		}}
	}

	return inst
}

func (srv *Server) terminateInstances(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	var resp struct {
		XMLName xml.Name
		ec2.TerminateInstancesResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "TerminateInstancesResponse"}
	resp.RequestId = reqId
	var insts []*Instance
	for attr, vals := range req.Form {
		if strings.HasPrefix(attr, "InstanceId.") {
			id := vals[0]
			inst := srv.instances[id]
			if inst == nil {
				fatalf(400, "InvalidInstanceID.NotFound", "no such instance id %q", id)
			}
			insts = append(insts, inst)
		}
	}
	for _, inst := range insts {
		// Delete any attached volumes that are "DeleteOnTermination"
		for _, va := range srv.volumeAttachments {
			if va.InstanceId != inst.id() || !va.DeleteOnTermination {
				continue
			}
			delete(srv.volumeAttachments, va.VolumeId)
			delete(srv.volumes, va.VolumeId)
		}
		resp.StateChanges = append(resp.StateChanges, inst.terminate())
	}
	return &resp
}

func (srv *Server) describeInstances(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	insts := make(map[*Instance]bool)
	for name, vals := range req.Form {
		if !strings.HasPrefix(name, "InstanceId.") {
			continue
		}
		inst := srv.instances[vals[0]]
		if inst == nil {
			fatalf(400, "InvalidInstanceID.NotFound", "instance %q not found", vals[0])
		}
		insts[inst] = true
	}

	f := newFilter(req.Form)

	var resp struct {
		XMLName xml.Name
		ec2.InstancesResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "DescribeInstancesResponse"}
	resp.RequestId = reqId
	for _, r := range srv.reservations {
		var instances []ec2.Instance
		var groups []ec2.SecurityGroup
		for _, g := range r.groups {
			groups = append(groups, g.ec2SecurityGroup())
		}
		for _, inst := range r.instances {
			if len(insts) > 0 && !insts[inst] {
				continue
			}
			// make instances in state "shutting-down" to transition
			// to "terminated" first, so we can simulate: shutdown,
			// subsequent refresh of the state with Instances(),
			// terminated.
			if inst.state == ShuttingDown {
				inst.state = Terminated
			}

			ok, err := f.ok(inst)
			if ok {
				instance := inst.ec2instance()
				instance.SecurityGroups = groups
				instances = append(instances, instance)
			} else if err != nil {
				fatalf(400, "InvalidParameterValue", "describe instances: %v", err)
			}
		}
		if len(instances) > 0 {
			resp.Reservations = append(resp.Reservations, ec2.Reservation{
				ReservationId:  r.id,
				OwnerId:        ownerId,
				Instances:      instances,
				SecurityGroups: groups,
			})
		}
	}
	return &resp
}

func (srv *Server) modifyInstanceAttribute(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	var inst *Instance
	var blockDeviceMappings []ec2.InstanceBlockDeviceMapping
	var sourceDestCheck *bool

	for attr, vals := range req.Form {
		if strings.HasPrefix(attr, "BlockDeviceMapping.") {
			fields := strings.SplitN(attr, ".", 3)
			if len(fields) != 3 {
				fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
			}
			i := atoi(fields[1])
			for i >= len(blockDeviceMappings) {
				blockDeviceMappings = append(
					blockDeviceMappings, ec2.InstanceBlockDeviceMapping{},
				)
			}
			switch fields[2] {
			case "DeviceName":
				blockDeviceMappings[i].DeviceName = vals[0]
			case "Ebs.DeleteOnTermination":
				val, err := strconv.ParseBool(vals[0])
				if err != nil {
					fatalf(400, "InvalidParameterValue", "bad flag %s: %s", attr, vals[0])
				}
				blockDeviceMappings[i].DeleteOnTermination = val
			case "Ebs.VolumeId":
				blockDeviceMappings[i].VolumeId = vals[0]
			default:
				fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
			}
			continue
		}
		switch attr {
		case "AWSAccessKeyId", "Action", "Signature", "SignatureMethod", "SignatureVersion",
			"Version", "Timestamp":
			continue
		case "InstanceId":
			inst = srv.instance(vals[0])
		case "SourceDestCheck.Value":
			val, err := strconv.ParseBool(vals[0])
			if err != nil {
				fatalf(400, "InvalidParameterValue", "bad flag %s: %s", attr, vals[0])
			}
			sourceDestCheck = &val
		default:
			fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
		}
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()

	switch {
	case sourceDestCheck != nil:
		if inst.vpcId == "" {
			fatalf(400, "InvalidParameterCombination", "You may only modify the sourceDestCheck attribute for VPC instances")
		}
		inst.sourceDestCheck = *sourceDestCheck
	case blockDeviceMappings != nil:
		for _, m := range blockDeviceMappings {
			for _, a := range srv.volumeAttachments {
				if a.InstanceId != inst.id() || a.Device != m.DeviceName {
					continue
				}
				a.DeleteOnTermination = m.DeleteOnTermination
			}
		}
	}

	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "ModifyInstanceAttributeResponse"},
		RequestId: reqId,
		Return:    true,
	}
}

func (srv *Server) instance(id string) *Instance {
	if id == "" {
		fatalf(400, "MissingParameter", "missing instanceId")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	inst, found := srv.instances[id]
	if !found {
		fatalf(400, "InvalidInstanceID.NotFound", "instance %s not found", id)
	}
	return inst
}

// runInstances implements the EC2 RunInstances entry point.
func (srv *Server) runInstances(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	min := atoi(req.Form.Get("MinCount"))
	max := atoi(req.Form.Get("MaxCount"))
	if min < 0 || max < 1 {
		fatalf(400, "InvalidParameterValue", "bad values for MinCount or MaxCount")
	}
	if min > max {
		fatalf(400, "InvalidParameterCombination", "MinCount is greater than MaxCount")
	}
	var userData []byte
	if data := req.Form.Get("UserData"); data != "" {
		var err error
		userData, err = b64.DecodeString(data)
		if err != nil {
			fatalf(400, "InvalidParameterValue", "bad UserData value: %v", err)
		}
	}

	// TODO attributes still to consider:
	//    ImageId:                  accept anything, we can verify later
	//    KeyName                   ?
	//    InstanceType              ?
	//    KernelId                  ?
	//    RamdiskId                 ?
	//    AvailZone                 ?
	//    GroupName                 tag
	//    Monitoring                ignore?
	//    DisableAPITermination     bool
	//    ShutdownBehavior          string
	//    PrivateIPAddress          string

	srv.mu.Lock()
	defer srv.mu.Unlock()

	// make sure that form fields are correct before creating the reservation.
	instType := req.Form.Get("InstanceType")
	imageId := req.Form.Get("ImageId")
	availZone := req.Form.Get("Placement.AvailabilityZone")
	if availZone == "" {
		availZone = defaultAvailZone
	}

	r := srv.newReservation(srv.formToGroups(req.Form))

	// If the user specifies an explicit subnet id, use it.
	// Otherwise, get a subnet from the default VPC.
	userSubnetId := req.Form.Get("SubnetId")
	instSubnet := srv.subnets[userSubnetId]
	if instSubnet == nil && userSubnetId != "" {
		fatalf(400, "InvalidSubnetID.NotFound", "subnet %s not found", userSubnetId)
	}
	if userSubnetId == "" {
		instSubnet = srv.getDefaultSubnet()
	}

	// Handle network interfaces parsing.
	ifacesToCreate, limitToOneInstance := srv.parseRunNetworkInterfaces(req)
	if len(ifacesToCreate) > 0 && userSubnetId != "" {
		// Since we have an instance-level subnet id
		// specified, we cannot add network interfaces
		// in the same request. See http://goo.gl/9aqbT9.
		fatalf(400, "InvalidParameterCombination", "Network interfaces and an instance-level subnet ID may not be specified on the same request")
	}
	if limitToOneInstance {
		max = 1
	}
	if len(ifacesToCreate) == 0 {
		// No NICs specified, so create a default one to simulate what
		// EC2 does.
		ifacesToCreate = srv.addDefaultNIC(instSubnet)
	}

	// Handle block device mappings.
	blockDeviceMappings := srv.parseBlockDeviceMappings(req)

	var resp struct {
		XMLName xml.Name
		ec2.RunInstancesResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "RunInstancesResponse"}
	resp.RequestId = reqId
	resp.ReservationId = r.id
	resp.OwnerId = ownerId

	for i := 0; i < max; i++ {
		inst := srv.newInstance(r, instType, imageId, availZone, srv.initialInstanceState)
		// Create any NICs on the instance subnet (if any), and then
		// save the VPC and subnet ids on the instance, as EC2 does.
		inst.ifaces = srv.createNICsOnRun(inst.id(), instSubnet, ifacesToCreate)
		if instSubnet != nil {
			inst.subnetId = instSubnet.Id
			inst.vpcId = instSubnet.VPCId
		}
		inst.UserData = userData
		inst.blockDeviceMappings = append(inst.blockDeviceMappings,
			srv.createBlockDeviceMappingsOnRun(inst.id(), blockDeviceMappings)...,
		)
		resp.Instances = append(resp.Instances, inst.ec2instance())
	}
	return &resp
}
