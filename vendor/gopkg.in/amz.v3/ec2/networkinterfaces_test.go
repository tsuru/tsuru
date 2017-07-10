//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2014 Canonical Ltd.
//

package ec2_test

import (
	"strings"
	"time"

	. "gopkg.in/check.v1"

	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/ec2"
)

// Network interface tests with example responses

func (s *S) TestCreateNetworkInterfaceExample(c *C) {
	testServer.Response(200, nil, CreateNetworkInterfaceExample)

	resp, err := s.ec2.CreateNetworkInterface(ec2.CreateNetworkInterface{
		SubnetId: "subnet-b2a249da",
		PrivateIPs: []ec2.PrivateIP{
			{Address: "10.0.2.157", IsPrimary: true},
		},
		SecurityGroupIds: []string{"sg-1a2b3c4d"},
	})
	req := testServer.WaitRequest()

	c.Assert(req.Form["Action"], DeepEquals, []string{"CreateNetworkInterface"})
	c.Assert(req.Form["SubnetId"], DeepEquals, []string{"subnet-b2a249da"})
	c.Assert(req.Form["PrivateIpAddress"], HasLen, 0)
	c.Assert(
		req.Form["PrivateIpAddresses.1.PrivateIpAddress"],
		DeepEquals,
		[]string{"10.0.2.157"},
	)
	c.Assert(
		req.Form["PrivateIpAddresses.1.Primary"],
		DeepEquals,
		[]string{"true"},
	)
	c.Assert(req.Form["Description"], HasLen, 0)
	c.Assert(req.Form["SecurityGroupId.1"], DeepEquals, []string{"sg-1a2b3c4d"})

	c.Assert(err, IsNil)
	c.Assert(resp.RequestId, Equals, "8dbe591e-5a22-48cb-b948-dd0aadd55adf")
	iface := resp.NetworkInterface
	c.Check(iface.Id, Equals, "eni-cfca76a6")
	c.Check(iface.SubnetId, Equals, "subnet-b2a249da")
	c.Check(iface.VPCId, Equals, "vpc-c31dafaa")
	c.Check(iface.AvailZone, Equals, "ap-southeast-1b")
	c.Check(iface.Description, Equals, "")
	c.Check(iface.OwnerId, Equals, "251839141158")
	c.Check(iface.RequesterManaged, Equals, false)
	c.Check(iface.Status, Equals, "available")
	c.Check(iface.MACAddress, Equals, "02:74:b0:72:79:61")
	c.Check(iface.PrivateIPAddress, Equals, "10.0.2.157")
	c.Check(iface.SourceDestCheck, Equals, true)
	c.Check(iface.Groups, DeepEquals, []ec2.SecurityGroup{
		{Id: "sg-1a2b3c4d", Name: "default"},
	})
	c.Check(iface.Tags, HasLen, 0)
	c.Check(iface.PrivateIPs, DeepEquals, []ec2.PrivateIP{
		{Address: "10.0.2.157", IsPrimary: true},
	})
}

func (s *S) TestDeleteNetworkInterfaceExample(c *C) {
	testServer.Response(200, nil, DeleteNetworkInterfaceExample)

	resp, err := s.ec2.DeleteNetworkInterface("eni-id")
	req := testServer.WaitRequest()

	c.Assert(req.Form["Action"], DeepEquals, []string{"DeleteNetworkInterface"})
	c.Assert(req.Form["NetworkInterfaceId"], DeepEquals, []string{"eni-id"})

	c.Assert(err, IsNil)
	c.Assert(resp.RequestId, Equals, "e1c6d73b-edaa-4e62-9909-6611404e1739")
}

func (s *S) TestNetworkInterfacesExample(c *C) {
	testServer.Response(200, nil, DescribeNetworkInterfacesExample)

	ids := []string{"eni-0f62d866", "eni-a66ed5cf"}
	resp, err := s.ec2.NetworkInterfaces(ids, nil)
	req := testServer.WaitRequest()

	c.Assert(req.Form["Action"], DeepEquals, []string{"DescribeNetworkInterfaces"})
	c.Assert(req.Form["NetworkInterfaceId.1"], DeepEquals, []string{ids[0]})
	c.Assert(req.Form["NetworkInterfaceId.2"], DeepEquals, []string{ids[1]})

	c.Assert(err, IsNil)
	c.Assert(resp.RequestId, Equals, "fc45294c-006b-457b-bab9-012f5b3b0e40")
	c.Check(resp.Interfaces, HasLen, 2)
	iface := resp.Interfaces[0]
	c.Check(iface.Id, Equals, ids[0])
	c.Check(iface.SubnetId, Equals, "subnet-c53c87ac")
	c.Check(iface.VPCId, Equals, "vpc-cc3c87a5")
	c.Check(iface.AvailZone, Equals, "ap-southeast-1b")
	c.Check(iface.Description, Equals, "")
	c.Check(iface.OwnerId, Equals, "053230519467")
	c.Check(iface.RequesterManaged, Equals, false)
	c.Check(iface.Status, Equals, "in-use")
	c.Check(iface.MACAddress, Equals, "02:81:60:cb:27:37")
	c.Check(iface.PrivateIPAddress, Equals, "10.0.0.146")
	c.Check(iface.SourceDestCheck, Equals, true)
	c.Check(iface.Groups, DeepEquals, []ec2.SecurityGroup{
		{Id: "sg-3f4b5653", Name: "default"},
	})
	c.Check(iface.Attachment, DeepEquals, ec2.NetworkInterfaceAttachment{
		Id:                  "eni-attach-6537fc0c",
		InstanceId:          "i-22197876",
		InstanceOwnerId:     "053230519467",
		DeviceIndex:         0,
		Status:              "attached",
		AttachTime:          "2012-07-01T21:45:27.000Z",
		DeleteOnTermination: true,
	})
	c.Check(iface.PrivateIPs, DeepEquals, []ec2.PrivateIP{
		{Address: "10.0.0.146", IsPrimary: true},
		{Address: "10.0.0.148", IsPrimary: false},
		{Address: "10.0.0.150", IsPrimary: false},
	})
	c.Check(iface.Tags, HasLen, 0)

	iface = resp.Interfaces[1]
	c.Check(iface.Id, Equals, ids[1])
	c.Check(iface.SubnetId, Equals, "subnet-cd8a35a4")
	c.Check(iface.VPCId, Equals, "vpc-f28a359b")
	c.Check(iface.AvailZone, Equals, "ap-southeast-1b")
	c.Check(iface.Description, Equals, "Primary network interface")
	c.Check(iface.OwnerId, Equals, "053230519467")
	c.Check(iface.RequesterManaged, Equals, false)
	c.Check(iface.Status, Equals, "in-use")
	c.Check(iface.MACAddress, Equals, "02:78:d7:00:8a:1e")
	c.Check(iface.PrivateIPAddress, Equals, "10.0.1.233")
	c.Check(iface.SourceDestCheck, Equals, true)
	c.Check(iface.Groups, DeepEquals, []ec2.SecurityGroup{
		{Id: "sg-a2a0b2ce", Name: "quick-start-1"},
	})
	c.Check(iface.Attachment, DeepEquals, ec2.NetworkInterfaceAttachment{
		Id:                  "eni-attach-a99c57c0",
		InstanceId:          "i-886401dc",
		InstanceOwnerId:     "053230519467",
		DeviceIndex:         0,
		Status:              "attached",
		AttachTime:          "2012-06-27T20:08:44.000Z",
		DeleteOnTermination: true,
	})
	c.Check(iface.PrivateIPs, DeepEquals, []ec2.PrivateIP{
		{Address: "10.0.1.233", IsPrimary: true},
		{Address: "10.0.1.20", IsPrimary: false},
	})
	c.Check(iface.Tags, HasLen, 0)
}

func (s *S) TestAttachNetworkInterfaceExample(c *C) {
	testServer.Response(200, nil, AttachNetworkInterfaceExample)

	resp, err := s.ec2.AttachNetworkInterface("eni-id", "i-id", 0)
	req := testServer.WaitRequest()

	c.Assert(req.Form["Action"], DeepEquals, []string{"AttachNetworkInterface"})
	c.Assert(req.Form["NetworkInterfaceId"], DeepEquals, []string{"eni-id"})
	c.Assert(req.Form["InstanceId"], DeepEquals, []string{"i-id"})
	c.Assert(req.Form["DeviceIndex"], DeepEquals, []string{"0"})

	c.Assert(err, IsNil)
	c.Assert(resp.RequestId, Equals, "ace8cd1e-e685-4e44-90fb-92014d907212")
	c.Assert(resp.AttachmentId, Equals, "eni-attach-d94b09b0")
}

func (s *S) TestDetachNetworkInterfaceExample(c *C) {
	testServer.Response(200, nil, DetachNetworkInterfaceExample)

	resp, err := s.ec2.DetachNetworkInterface("eni-attach-id", true)
	req := testServer.WaitRequest()

	c.Assert(req.Form["Action"], DeepEquals, []string{"DetachNetworkInterface"})
	c.Assert(req.Form["AttachmentId"], DeepEquals, []string{"eni-attach-id"})
	c.Assert(req.Form["Force"], DeepEquals, []string{"true"})

	c.Assert(err, IsNil)
	c.Assert(resp.RequestId, Equals, "ce540707-0635-46bc-97da-33a8a362a0e8")
}

// Network interface tests run against either a local test server or
// live on EC2.

func (s *ServerTests) TestNetworkInterfaces(c *C) {
	// This tests CreateNetworkInterface, DeleteNetworkInterface,
	// AttachNetworkInterface, DetachNetworkInterface, and basic
	// retrieval using NetworkInterfaces. Filtering is tested
	// separately.
	results := s.prepareNetworkInterfaces(c)
	results.cleanup()
}

func (s *ServerTests) TestNetworkInterfacesFiltering(c *C) {
	// Test all filters supported by both Amazon and ec2test server.
	prep := s.prepareNetworkInterfaces(c)
	defer prep.cleanup()

	type filterTest struct {
		about      string
		ids        []string     // ids argument to NetworkInterfaces method.
		filters    []filterSpec // filters argument to NetworkInterfaces method.
		resultIds  []string     // expected ids of the results.
		allowExtra bool         // specified results may be incomplete.
		err        string       // expected error.
	}
	nicIds := func(ids ...int) []string {
		var resultIds []string
		for _, id := range ids {
			switch id {
			case 1:
				resultIds = append(resultIds, prep.nicId1)
			case 2:
				resultIds = append(resultIds, prep.nicId2)
			}
		}
		return resultIds
	}
	filterCheck := func(name, val string, resultIds []string) filterTest {
		vals := []string{val}
		if strings.ContainsAny(val, "|") {
			vals = strings.Split(val, "|")
		}
		return filterTest{
			about:      "filter check " + name + " = " + val,
			filters:    []filterSpec{{name, vals}},
			resultIds:  resultIds,
			allowExtra: true,
		}
	}
	tests := []filterTest{{
		about:      "no ids or filters returns all NICs",
		resultIds:  nicIds(1, 2),
		allowExtra: true,
	}, {
		about:     "two specified ids returns only them",
		ids:       nicIds(1, 2),
		resultIds: nicIds(1, 2),
	}, {
		about: "non-existent NIC id gives an error",
		ids:   []string{"eni-dddddddd"},
		err:   `.*\(InvalidNetworkInterfaceID\.NotFound\)`,
	}, {
		about:     "filter by network-interface-id with both ids",
		filters:   []filterSpec{{"network-interface-id", nicIds(1, 2)}},
		resultIds: nicIds(1, 2),
	}, {
		about:     "previous filter and ids gives the same result",
		ids:       nicIds(1, 2),
		filters:   []filterSpec{{"network-interface-id", nicIds(1, 2)}},
		resultIds: nicIds(1, 2),
	}, {
		about:   "filter by one id and specify another id in the list - no results",
		ids:     nicIds(1),
		filters: []filterSpec{{"network-interface-id", nicIds(2)}},
	}, {
		about: "combination filters: first NIC by description and id",
		filters: []filterSpec{
			{"description", []string{"My first iface"}},
			{"network-interface-id", nicIds(1)},
		},
		resultIds: nicIds(1),
	}, {
		about: "combination filters: both NICs by MAC address",
		filters: []filterSpec{
			{"mac-address", []string{prep.nic1MAC, prep.nic2MAC}},
		},
		resultIds: nicIds(1, 2),
	}, {
		about:   "filter by bogus private-dns-name - no results",
		filters: []filterSpec{{"private-dns-name", []string{"invalid"}}},
	},
		filterCheck("availability-zone", prep.availZone, nicIds(1, 2)),
		filterCheck("subnet-id", prep.subId, nicIds(1, 2)),
		filterCheck("vpc-id", prep.vpcId, nicIds(1, 2)),
		filterCheck("owner-id", prep.ownerId, nicIds(1, 2)),
		filterCheck("network-interface-id", prep.nicId1, nicIds(1)),
		filterCheck("status", "in-use", nicIds(2)),
		filterCheck("attachment.attachment-id", prep.attId, nicIds(2)),
		filterCheck("attachment.instance-id", prep.instId, nicIds(2)),
		filterCheck("attachment.instance-owner-id", prep.ownerId, nicIds(2)),
		filterCheck("attachment.device-index", "1", nicIds(2)),
		filterCheck("attachment.status", "attached|attaching", nicIds(2)),
		filterCheck("attachment.delete-on-termination", "false", nicIds(2)),
		filterCheck("attachment.attach-time", prep.nic2AttachTime, nicIds(2)),
		filterCheck("source-dest-check", "true", nicIds(1, 2)),
		filterCheck("description", "My first iface", nicIds(1)),
		filterCheck("private-ip-address", prep.ips1[0].Address, nicIds(1)),
		filterCheck("addresses.private-ip-address", prep.ips2[1].Address, nicIds(2)),
		filterCheck("addresses.primary", "true", nicIds(1, 2)),
		filterCheck("group-id", prep.group.Id, nicIds(2)),
		filterCheck("group-name", prep.group.Name, nicIds(2)),
	}
	for i, t := range tests {
		c.Logf("%d. %s", i, t.about)
		f := ec2.NewFilter()
		if t.filters != nil {
			for _, spec := range t.filters {
				f.Add(spec.name, spec.values...)
			}
		}
		c.Logf("using filter: %v", f)
		c.Logf("expecing ids: %v\n", t.resultIds)
		resp, err := s.ec2.NetworkInterfaces(t.ids, f)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err)
			continue
		}
		if !c.Check(err, IsNil) {
			continue
		}
		nics := make(map[string]ec2.NetworkInterface)
		for _, nic := range resp.Interfaces {
			_, found := nics[nic.Id]
			c.Check(found, Equals, false, Commentf("duplicate NIC id: %q", nic.Id))
			nics[nic.Id] = nic
		}
		// Extra NICs may be returned, so eliminate all NICs that we
		// did not create in this session.
		if t.allowExtra {
			// Don't range over a map while deleting from it!
			noExtraNICs := make(map[string]ec2.NetworkInterface)
			for id, nic := range nics {
				if id == prep.nicId1 || id == prep.nicId2 {
					noExtraNICs[id] = nic
				} else {
					c.Logf("filtering extra NIC %v", nic)
				}
			}
			nics = noExtraNICs
		}
		c.Check(
			nics, HasLen, len(t.resultIds),
			Commentf("expected %d NICs, got %d", len(t.resultIds), len(nics)),
		)
		for j, nicId := range t.resultIds {
			nic := nics[nicId]
			if c.Check(nic, NotNil, Commentf("NIC %d (%v) not found; got %#v", j, nicId, nics)) {
				c.Check(nic.Id, Equals, nicId, Commentf("NIC %d (%v)", j, nicId))
			}
		}
	}
}

// prepareNICsResults holds the necessary information to use what
// prepareNetworkInterfaces has created and clean it up afterwards.
type prepareNICsResults struct {
	vpcId          string
	subId          string
	group          ec2.SecurityGroup
	instId         string
	nicId1         string
	nic1MAC        string
	nicId2         string
	nic2MAC        string
	nic2DNS        string
	ips1           []ec2.PrivateIP
	ips2           []ec2.PrivateIP
	attId          string
	nic2AttachTime string
	availZone      string
	ownerId        string

	cleanup func()
}

// prepareNetworkInterfaces creates a VPC, a subnet, and a security
// group. Then launches a t1.micro instance and creates 2 network
// interfaces - first one with a single private IP, second one with 2
// private IPs. Waits until the interfaces were created and then
// attaches the second one to the instance. Finally, it returns a
// populated prepareNICsResults with the created entities and a
// cleanup callback.
func (s *ServerTests) prepareNetworkInterfaces(c *C) (results prepareNICsResults) {
	vpcResp, err := s.ec2.CreateVPC("10.3.0.0/16", "")
	c.Assert(err, IsNil)
	results.vpcId = vpcResp.VPC.Id

	subResp := s.createSubnet(c, results.vpcId, "10.3.1.0/24", "")
	results.subId = subResp.Subnet.Id
	results.group = s.makeTestGroup(c, results.vpcId, "vpc-sg-1", "vpc test group1")

	instList, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
		SubnetId:     results.subId,
	})
	c.Assert(err, IsNil)
	inst := instList.Instances[0]
	c.Assert(inst, NotNil)
	results.instId = inst.InstanceId

	results.ips1 = []ec2.PrivateIP{{
		Address:   "10.3.1.10",
		DNSName:   "ip-10-3-1-10.ec2.internal",
		IsPrimary: true,
	}}
	resp1, err := s.ec2.CreateNetworkInterface(ec2.CreateNetworkInterface{
		SubnetId:    results.subId,
		PrivateIPs:  results.ips1,
		Description: "My first iface",
	})
	c.Assert(err, IsNil)
	c.Logf("created NIC %v", resp1.NetworkInterface)
	assertNetworkInterface(c, resp1.NetworkInterface, "", results.subId, results.ips1)
	c.Check(resp1.NetworkInterface.Description, Equals, "My first iface")
	results.nicId1 = resp1.NetworkInterface.Id
	results.nic1MAC = resp1.NetworkInterface.MACAddress
	// These two are the same for both NICs, so set them once only.
	results.availZone = resp1.NetworkInterface.AvailZone
	results.ownerId = resp1.NetworkInterface.OwnerId

	results.ips2 = []ec2.PrivateIP{{
		Address:   "10.3.1.20",
		DNSName:   "ip-10-3-1-20.ec2.internal",
		IsPrimary: true,
	}, {
		Address:   "10.3.1.22",
		DNSName:   "ip-10-3-1-22.ec2.internal",
		IsPrimary: false,
	}}
	resp2, err := s.ec2.CreateNetworkInterface(ec2.CreateNetworkInterface{
		SubnetId:         results.subId,
		PrivateIPs:       results.ips2,
		SecurityGroupIds: []string{results.group.Id},
	})
	c.Assert(err, IsNil)
	c.Logf("created NIC %v", resp2.NetworkInterface)
	assertNetworkInterface(c, resp2.NetworkInterface, "", results.subId, results.ips2)
	c.Assert(resp2.NetworkInterface.Groups, DeepEquals, []ec2.SecurityGroup{results.group})
	results.nicId2 = resp2.NetworkInterface.Id
	results.nic2MAC = resp2.NetworkInterface.MACAddress
	results.nic2DNS = resp2.NetworkInterface.PrivateDNSName

	// We only check for the network interfaces we just created,
	// because the user might have others in his account (when testing
	// against the EC2 servers). In some cases it takes a short while
	// until both interfaces are created, so we need to retry a few
	// times to make sure.
	testAttempt := aws.AttemptStrategy{
		Total: 5 * time.Minute,
		Delay: 5 * time.Second,
	}
	var list *ec2.NetworkInterfacesResp
	done := false
	for a := testAttempt.Start(); a.Next(); {
		c.Logf("waiting for %v to be created", []string{results.nicId1, results.nicId2})
		list, err = s.ec2.NetworkInterfaces(nil, nil)
		if err != nil {
			c.Logf("retrying; NetworkInterfaces returned: %v", err)
			continue
		}
		found := 0
		for _, iface := range list.Interfaces {
			c.Logf("found NIC %v", iface)
			switch iface.Id {
			case results.nicId1:
				assertNetworkInterface(c, iface, results.nicId1, results.subId, results.ips1)
				found++
			case results.nicId2:
				assertNetworkInterface(c, iface, results.nicId2, results.subId, results.ips2)
				found++
			}
			if found == 2 {
				done = true
				break
			}
		}
		if done {
			c.Logf("all NICs were created and attached")
			break
		}
	}
	if !done {
		// Attachment failed, but we still need to clean up.
		results.cleanup = func() {
			terminateInstances(c, s.ec2, []string{results.instId})
			s.deleteInterfaces(c, []string{results.nicId1, results.nicId2})
			s.deleteGroups(c, []ec2.SecurityGroup{results.group})
			s.deleteSubnets(c, []string{results.subId})
			s.deleteVPCs(c, []string{results.vpcId})
		}
		c.Fatalf("timeout while waiting for NICs %v", []string{results.nicId1, results.nicId2})
		return
	}

	list, err = s.ec2.NetworkInterfaces([]string{results.nicId1}, nil)
	c.Assert(err, IsNil)
	c.Assert(list.Interfaces, HasLen, 1)
	assertNetworkInterface(c, list.Interfaces[0], results.nicId1, results.subId, results.ips1)

	f := ec2.NewFilter()
	f.Add("network-interface-id", results.nicId2)
	list, err = s.ec2.NetworkInterfaces(nil, f)
	c.Assert(err, IsNil)
	c.Assert(list.Interfaces, HasLen, 1)
	assertNetworkInterface(c, list.Interfaces[0], results.nicId2, results.subId, results.ips2)

	// Attachment might fail if the instance is not running yet,
	// so we retry for a while until it succeeds.
	c.Logf("attaching NIC %q to instance %q...", results.nicId2, results.instId)
	var attResp *ec2.AttachNetworkInterfaceResp
	for a := testAttempt.Start(); a.Next(); {
		attResp, err = s.ec2.AttachNetworkInterface(results.nicId2, results.instId, 1)
		if err != nil {
			c.Logf("AttachNetworkInterface returned: %v; retrying...", err)
			attResp = nil
			continue
		}
		c.Logf("AttachNetworkInterface succeeded")
		c.Check(attResp.AttachmentId, Not(Equals), "")
		break
	}
	if attResp == nil {
		c.Fatalf("timeout while waiting for AttachNetworkInterface to succeed")
	}

	list, err = s.ec2.NetworkInterfaces([]string{results.nicId2}, nil)
	c.Assert(err, IsNil)
	att := list.Interfaces[0].Attachment
	c.Check(att.Id, Equals, attResp.AttachmentId)
	c.Check(att.InstanceId, Equals, results.instId)
	c.Check(att.DeviceIndex, Equals, 1)
	c.Check(att.Status, Matches, "(attaching|attached)")
	results.attId = att.Id
	results.nic2AttachTime = att.AttachTime

	results.cleanup = func() {
		terminateInstances(c, s.ec2, []string{results.instId})

		// We won't be able to delete the interface until it is attached,
		// so detach it first.
		c.Logf("detaching NIC %q from instance %q...", results.nicId2, results.instId)
		for a := testAttempt.Start(); a.Next(); {
			_, err := s.ec2.DetachNetworkInterface(results.attId, true)
			if err != nil {
				c.Logf("DetachNetworkInterface returned: %v; retrying...", err)
				continue
			}
			c.Logf("DetachNetworkInterface succeeded")
			break
		}

		s.deleteInterfaces(c, []string{results.nicId1, results.nicId2})
		s.deleteGroups(c, []ec2.SecurityGroup{results.group})
		s.deleteSubnets(c, []string{results.subId})
		s.deleteVPCs(c, []string{results.vpcId})
	}
	return results
}

// deleteInterfaces ensures the given network interfaces are deleted,
// by retrying until a timeout or all interfaces cannot be found
// anymore. This should be used to make sure tests leave no interfaces
// around.
func (s *ServerTests) deleteInterfaces(c *C, ids []string) {
	testAttempt := aws.AttemptStrategy{
		Total: 5 * time.Minute,
		Delay: 5 * time.Second,
	}
	for a := testAttempt.Start(); a.Next(); {
		deleted := 0
		c.Logf("deleting interfaces %v", ids)
		for _, id := range ids {
			_, err := s.ec2.DeleteNetworkInterface(id)
			if err == nil || errorCode(err) == "InvalidNetworkInterfaceID.NotFound" {
				c.Logf("interface %s deleted", id)
				deleted++
				continue
			}
			if err != nil {
				c.Logf("retrying; DeleteNetworkInterface returned: %v", err)
			}
		}
		if deleted == len(ids) {
			c.Logf("all interfaces deleted")
			return
		}
	}
	c.Fatalf("timeout while waiting %v interfaces to get deleted!", ids)
}

func assertNetworkInterface(c *C, obtained ec2.NetworkInterface, expectId, expectSubId string, expectIPs []ec2.PrivateIP) {
	if expectId != "" {
		c.Check(obtained.Id, Equals, expectId)
	} else {
		c.Check(obtained.Id, Matches, `^eni-[0-9a-f]+$`)
	}
	c.Check(obtained.Status, Matches, "(available|pending|in-use)")
	if expectSubId != "" {
		c.Check(obtained.SubnetId, Equals, expectSubId)
	} else {
		c.Check(obtained.SubnetId, Matches, `^subnet-[0-9a-f]+$`)
	}
	c.Check(obtained.Attachment, DeepEquals, ec2.NetworkInterfaceAttachment{})
	if len(expectIPs) > 0 {
		c.Check(obtained.PrivateIPs, HasLen, len(expectIPs))
		// AWS does not always set DNSName right after NIC creation,
		// so we only check the obtained DNSName only if not empty.
		for i, _ := range expectIPs {
			if obtained.PrivateIPs[i].DNSName == "" {
				// AWS didn't report it yet, so we also set the
				// expected DNSName for this IP to empty to ensure the
				// DeepEquals check below has a chance to pass.
				c.Logf("obtained.PrivateIPs[%d].DNSName is empty", i)
				expectIPs[i].DNSName = ""
			} else if expectIPs[i].DNSName == "" {
				// Allow DeepEquals to succeed below: since DNSName is
				// not expected on this IP, set it to whatever we
				// obtained.
				dnsName := obtained.PrivateIPs[i].DNSName
				c.Logf("obtained.PrivateIPs[%d].DNSName is %q", i, dnsName)
				expectIPs[i].DNSName = dnsName
			}
		}
		c.Check(obtained.PrivateIPs, DeepEquals, expectIPs)
		c.Check(obtained.PrivateIPAddress, DeepEquals, expectIPs[0].Address)
	}
}
