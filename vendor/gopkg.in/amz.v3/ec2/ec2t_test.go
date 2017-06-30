package ec2_test

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"time"

	. "gopkg.in/check.v1"

	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/ec2/ec2test"
	"gopkg.in/amz.v3/testutil"
)

// defaultAvailZone is the availability zone to use by default when
// launching instances during live tests. Because t1.micro/m1.medium
// instance types are not available (as of 2014-10-01) in some zones,
// we use us-east-1c as t1.micro/m1.medium are still available there.
const defaultAvailZone = "us-east-1c"

// LocalServer represents a local ec2test fake server.
type LocalServer struct {
	auth   aws.Auth
	region aws.Region
	srv    *ec2test.Server
}

func (s *LocalServer) SetUp(c *C) {
	srv, err := ec2test.NewServer()
	c.Assert(err, IsNil)
	c.Assert(srv, NotNil)
	srv.SetCreateRootDisks(true)

	// Set up default VPC.
	_, err = srv.AddDefaultVPCAndSubnets()
	c.Assert(err, IsNil)
	s.srv = srv
	s.region = aws.Region{EC2Endpoint: srv.URL()}
}

// LocalServerSuite defines tests that will run
// against the local ec2test server. It includes
// selected tests from ClientTests;
// when the ec2test functionality is sufficient, it should
// include all of them, and ClientTests can be simply embedded.
type LocalServerSuite struct {
	srv LocalServer
	ServerTests
	clientTests ClientTests
}

var _ = Suite(&LocalServerSuite{})

func (s *LocalServerSuite) SetUpSuite(c *C) {
	s.srv.SetUp(c)
	s.ServerTests.ec2 = ec2.New(s.srv.auth, s.srv.region, aws.SignV2)
	s.clientTests.ec2 = ec2.New(s.srv.auth, s.srv.region, aws.SignV2)
}

func (s *LocalServerSuite) TestRunAndTerminate(c *C) {
	s.clientTests.TestRunAndTerminate(c)
}

func (s *LocalServerSuite) TestSecurityGroups(c *C) {
	s.clientTests.TestSecurityGroups(c)
}

func (s *LocalServerSuite) TestVolumeAttachments(c *C) {
	s.srv.srv.SetInitialInstanceState(ec2test.Running)
	s.ServerTests.testVolumeAttachments(c)
}

func (s *LocalServerSuite) TestModifyInstanceAttributeBlockDeviceMappings(c *C) {
	s.srv.srv.SetInitialInstanceState(ec2test.Running)
	s.ServerTests.testModifyInstanceAttributeBlockDeviceMappings(c)
}

func (s *LocalServerSuite) TestModifyInstanceAttributeSourceDestCheck(c *C) {
	s.srv.srv.SetInitialInstanceState(ec2test.Running)
	s.ServerTests.testModifyInstanceAttributeSourceDestCheck(c)
}

// TestUserData is not defined on ServerTests because it
// requires the ec2test server to function.
func (s *LocalServerSuite) TestUserData(c *C) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	inst, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
		UserData:     data,
		AvailZone:    defaultAvailZone,
	})
	c.Assert(err, IsNil)
	c.Assert(inst, NotNil)

	id := inst.Instances[0].InstanceId

	defer terminateInstances(c, s.ec2, []string{id})

	tinst := s.srv.srv.Instance(id)
	c.Assert(tinst, NotNil)
	c.Assert(tinst.UserData, DeepEquals, data)
}

func (s *LocalServerSuite) TestInstanceInfo(c *C) {
	list, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
		AvailZone:    defaultAvailZone,
	})
	c.Assert(err, IsNil)

	inst := list.Instances[0]
	c.Assert(inst, NotNil)

	id := inst.InstanceId
	defer terminateInstances(c, s.ec2, []string{id})

	masked := func(addr string) string {
		return net.ParseIP(addr).Mask(net.CIDRMask(24, 32)).String()
	}
	c.Check(masked(inst.IPAddress), Equals, "8.0.0.0")
	c.Check(masked(inst.PrivateIPAddress), Equals, "127.0.0.0")
	// DNSName is empty initially, to check it we need to refresh.
	c.Check(inst.DNSName, Equals, "")
	c.Check(inst.PrivateDNSName, Equals, id+".internal.invalid")

	// Get the instance again to verify DNSName.
	resp, err := s.ec2.Instances([]string{id}, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Reservations, HasLen, 1)
	c.Assert(resp.Reservations[0].Instances, HasLen, 1)
	c.Check(resp.Reservations[0].Instances[0].DNSName, Equals, id+".testing.invalid")
}

// getDefaultVPCIdAndSubnets returns the default VPC id and a list of
// its default subnets. If fails if there is no default VPC or at
// least one subnet in it.
func (s *ServerTests) getDefaultVPCIdAndSubnets(c *C) (string, []ec2.Subnet) {
	resp1, err := s.ec2.AccountAttributes("default-vpc")
	c.Assert(err, IsNil)
	c.Assert(resp1.Attributes, HasLen, 1)
	c.Assert(resp1.Attributes[0].Name, Equals, "default-vpc")
	c.Assert(resp1.Attributes[0].Values, HasLen, 1)
	c.Assert(resp1.Attributes[0].Values[0], Not(Equals), "")
	defaultVPCId := resp1.Attributes[0].Values[0]
	if defaultVPCId == "none" {
		c.Fatalf("no default VPC for region %q", s.ec2.Region.Name)
	}
	filter := ec2.NewFilter()
	filter.Add("defaultForAz", "true")
	filter.Add("vpc-id", defaultVPCId)
	resp2, err := s.ec2.Subnets(nil, filter)
	c.Assert(err, IsNil)
	defaultSubnets := resp2.Subnets
	if len(defaultSubnets) < 1 {
		c.Fatalf("no default subnets for VPC %q", defaultVPCId)
	}

	return defaultVPCId, defaultSubnets
}

func validIPForSubnet(c *C, subnet ec2.Subnet, startFrom byte) string {
	ip, ipnet, err := net.ParseCIDR(subnet.CIDRBlock)
	c.Assert(err, IsNil)
	ip[len(ip)-1] = startFrom
	c.Assert(ipnet.Contains(ip), Equals, true)
	return ip.String()
}

func (s *LocalServerSuite) TestAvailabilityZones(c *C) {
	s.srv.srv.SetAvailabilityZones([]ec2.AvailabilityZoneInfo{{
		AvailabilityZone: ec2.AvailabilityZone{
			Name:   "us-east-1a",
			Region: "us-east-1",
		},
		State: "available",
	}, {
		AvailabilityZone: ec2.AvailabilityZone{
			Name:   "us-east-1b",
			Region: "us-east-1",
		},
		State: "impaired",
	}, {
		AvailabilityZone: ec2.AvailabilityZone{
			Name:   "us-west-1a",
			Region: "us-west-1",
		},
		State: "available",
	}, {
		AvailabilityZone: ec2.AvailabilityZone{
			Name:   "us-west-1b",
			Region: "us-west-1",
		},
		State:      "unavailable",
		MessageSet: []string{"down for maintenance"},
	}})

	resp, err := s.ec2.AvailabilityZones(nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Zones, HasLen, 4)
	for _, zone := range resp.Zones {
		c.Check(zone.Name, Matches, "^us-(east|west)-1[ab]$")
	}

	filter := ec2.NewFilter()
	filter.Add("region-name", "us-east-1")
	resp, err = s.ec2.AvailabilityZones(filter)
	c.Assert(err, IsNil)
	c.Assert(resp.Zones, HasLen, 2)
	for _, zone := range resp.Zones {
		c.Check(zone.Name, Matches, "^us-east-1[ab]$")
	}
}

// AmazonServerSuite runs the ec2test server tests against a live EC2 server.
// It will only be activated if the -amazon flag is specified.
type AmazonServerSuite struct {
	srv AmazonServer
	ServerTests
}

var _ = Suite(&AmazonServerSuite{})

func (s *AmazonServerSuite) SetUpSuite(c *C) {
	if !testutil.Amazon {
		c.Skip("AmazonServerSuite tests not enabled")
	}
	s.srv.SetUp(c)
	s.ServerTests.ec2 = ec2.New(s.srv.auth, aws.USEast, aws.SignV2)
}

// ServerTests defines a set of tests designed to test
// the ec2test local fake ec2 server.
// It is not used as a test suite in itself, but embedded within
// another type.
type ServerTests struct {
	ec2 *ec2.EC2
}

func (s *ServerTests) TestDescribeAccountAttributes(c *C) {
	resp, err := s.ec2.AccountAttributes("supported-platforms", "default-vpc")
	c.Assert(err, IsNil)
	c.Assert(resp.Attributes, HasLen, 2)
	for _, attr := range resp.Attributes {
		switch attr.Name {
		case "supported-platforms":
			sort.Strings(attr.Values)
			c.Assert(attr.Values, Not(HasLen), 0)
			if len(attr.Values) == 2 {
				c.Assert(attr.Values, DeepEquals, []string{"EC2", "VPC"})
			} else if len(attr.Values) == 1 {
				// Some regions have only VPC or EC2 enabled, and
				// because this test runs both against the local test
				// server and the Amazon live servers we need to
				// account for both cases.
				c.Assert(attr.Values[0], Matches, `(EC2|VPC)`)
			} else {
				c.Fatalf("unexpected account attributes: %v", attr.Values)
			}
		case "default-vpc":
			c.Assert(attr.Values, HasLen, 1)
			c.Assert(attr.Values[0], Not(Equals), "")
		default:
			c.Fatalf("unexpected account attribute %q: %v", attr.Name, attr)
		}
	}
}

func (s *ServerTests) getDefaultVPCSubnetSuitableForT1Micro(c *C) (defaultVPCId string, defaultSubnet ec2.Subnet) {
	defaultVPCId, defaultSubnets := s.getDefaultVPCIdAndSubnets(c)
	// We need to pick a subnet in AZ where t1-micro is still
	// supported, which means (as of 2015-08-07) us-east-1d,
	// us-east-1c, or us-east-1a, but *not* us-east-1e.
	var subnet ec2.Subnet
	for _, sub := range defaultSubnets {
		if sub.AvailZone != "us-east-1e" {
			subnet = sub
			break
		}
	}
	if subnet.Id == "" {
		// No luck, better report it and give up.
		c.Fatalf("cannot find a default VPC subnet not in AZ us-east-1e")
	}
	return defaultVPCId, subnet
}

func (s *ServerTests) TestRunInstancesVPCCreatesNICsWhenSpecified(c *C) {
	defaultVPCId, defaultSubnet := s.getDefaultVPCSubnetSuitableForT1Micro(c)
	g0 := s.makeTestGroup(c, defaultVPCId, "goamz-test0", "ec2test group 0")
	g1 := s.makeTestGroup(c, defaultVPCId, "goamz-test1", "ec2test group 1")
	defer s.deleteGroups(c, []ec2.SecurityGroup{g0, g1})

	ip1 := validIPForSubnet(c, defaultSubnet, 5)
	ip2 := validIPForSubnet(c, defaultSubnet, 6)
	list, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
		NetworkInterfaces: []ec2.RunNetworkInterface{{
			DeviceIndex:         0,
			SubnetId:            defaultSubnet.Id,
			Description:         "first nic",
			SecurityGroupIds:    []string{g0.Id, g1.Id},
			DeleteOnTermination: true,
			PrivateIPs: []ec2.PrivateIP{
				{Address: ip1, IsPrimary: true},
				{Address: ip2, IsPrimary: false},
			}}}})
	c.Assert(err, IsNil)

	inst := list.Instances[0]
	c.Assert(inst, NotNil)
	c.Assert(inst.NetworkInterfaces, HasLen, 1)

	id := inst.InstanceId
	defer terminateInstances(c, s.ec2, []string{id})

	c.Check(inst.VPCId, Equals, defaultSubnet.VPCId)
	c.Check(inst.SubnetId, Equals, defaultSubnet.Id)
	iface := inst.NetworkInterfaces[0]
	c.Check(iface.Id, Matches, "eni-.+")
	c.Check(iface.Attachment.DeviceIndex, Equals, 0)
	c.Check(iface.Attachment.Id, Matches, "eni-attach-.+")
	c.Check(iface.SubnetId, Equals, defaultSubnet.Id)
	c.Check(iface.PrivateIPAddress, Equals, ip1)
	c.Check(iface.Description, Equals, "first nic")
	c.Check(iface.Groups, HasLen, 2)
	for _, group := range iface.Groups {
		if group.Id == g0.Id {
			c.Check(group.Name, Equals, g0.Name)
		} else if group.Id == g1.Id {
			c.Check(group.Name, Equals, g1.Name)
		}
	}
	for _, ip := range iface.PrivateIPs {
		if ip.IsPrimary {
			c.Check(ip.Address, Equals, ip1)
		} else {
			c.Check(ip.Address, Equals, ip2)
		}
	}
}

func (s *ServerTests) TestRunInstancesVPCReturnsErrorWithBothInstanceAndNICSubnetIds(c *C) {
	_, defaultSubnet := s.getDefaultVPCSubnetSuitableForT1Micro(c)

	_, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
		SubnetId:     defaultSubnet.Id,
		NetworkInterfaces: []ec2.RunNetworkInterface{{
			DeviceIndex:         0,
			SubnetId:            defaultSubnet.Id,
			Description:         "first nic",
			DeleteOnTermination: true,
		}},
	})
	c.Assert(err, NotNil)
	c.Check(errorCode(err), Equals, "InvalidParameterCombination")
}

func (s *ServerTests) testCreateDefaultNIC(c *C, subnet *ec2.Subnet) {
	defaultVPCId, defaultSubnet := s.getDefaultVPCSubnetSuitableForT1Micro(c)

	params := &ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
	}
	if subnet != nil {
		params.SubnetId = subnet.Id
	} else {
		subnet = &defaultSubnet
		params.AvailZone = defaultSubnet.AvailZone
	}
	list, err := s.ec2.RunInstances(params)
	c.Assert(err, IsNil)

	inst := list.Instances[0]
	c.Assert(inst, NotNil)
	c.Assert(inst.NetworkInterfaces, HasLen, 1)

	id := inst.InstanceId
	defer terminateInstances(c, s.ec2, []string{id})

	c.Check(inst.VPCId, Equals, defaultVPCId)
	if inst.SubnetId != subnet.Id {
		// Since we don't specify which AZ to use,
		// the instance might launch in any one.
		defaultSubnetsResp, err := s.ec2.Subnets([]string{inst.SubnetId}, nil)
		c.Assert(err, IsNil)
		for _, sub := range defaultSubnetsResp.Subnets {
			if inst.SubnetId == sub.Id {
				subnet = &sub
				break
			}
		}
	}
	_, ipnet, err := net.ParseCIDR(subnet.CIDRBlock)
	c.Assert(err, IsNil)

	c.Check(inst.SubnetId, Equals, subnet.Id)
	iface := inst.NetworkInterfaces[0]
	c.Assert(iface.Id, Matches, "eni-.+")
	c.Assert(iface.SubnetId, Equals, subnet.Id)
	c.Assert(iface.VPCId, Equals, subnet.VPCId)
	if len(iface.PrivateIPs) > 0 {
		// AWS doesn't always fill in the PrivateIPs slice.
		expectIP := ec2.PrivateIP{
			Address:   iface.PrivateIPAddress,
			DNSName:   iface.PrivateDNSName,
			IsPrimary: true,
		}
		c.Assert(iface.PrivateIPs, DeepEquals, []ec2.PrivateIP{expectIP})
	}
	c.Assert(ipnet.Contains(net.ParseIP(iface.PrivateIPAddress)), Equals, true)
	c.Assert(iface.PrivateDNSName, Not(Equals), "")
	c.Assert(iface.Attachment.Id, Matches, "eni-attach-.+")
}

func (s *ServerTests) TestRunInstancesVPCCreatesDefaultNICWithoutSubnetIdOrNICs(c *C) {
	s.testCreateDefaultNIC(c, nil)
}

func (s *ServerTests) TestRunInstancesVPCCreatesDefaultNICWithSubnetIdNoNICs(c *C) {
	_, defaultSubnet := s.getDefaultVPCSubnetSuitableForT1Micro(c)
	s.testCreateDefaultNIC(c, &defaultSubnet)
}

func terminateInstances(c *C, e *ec2.EC2, ids []string) {
	_, err := e.TerminateInstances(ids)
	if err != nil && c.Check(errorCode(err), Equals, "InvalidInstanceID.NotFound") {
		// Nothing to do.
		return
	} else {
		c.Assert(err, IsNil, Commentf("%v INSTANCES LEFT RUNNING!!!", ids))
	}
	// We need to wait until the instances are really off, because
	// entities that depend on them won't be deleted (i.e. groups,
	// NICs, subnets, etc.)
	testAttempt := aws.AttemptStrategy{
		Total: 10 * time.Minute,
		Delay: 5 * time.Second,
	}
	f := ec2.NewFilter()
	f.Add("instance-state-name", "terminated")
	idsLeft := make(map[string]bool)
	for _, id := range ids {
		idsLeft[id] = true
	}
	for a := testAttempt.Start(); a.Next(); {
		c.Logf("waiting for %v to get terminated", ids)
		resp, err := e.Instances(ids, f)
		if err != nil && errorCode(err) == "InvalidInstanceID.NotFound" {
			c.Logf("all instances terminated.")
			return
		} else if err != nil {
			c.Fatalf("not waiting for %v to terminate: %v", ids, err)
		}
		for _, r := range resp.Reservations {
			for _, inst := range r.Instances {
				delete(idsLeft, inst.InstanceId)
			}
		}
		ids = []string{}
		for id, _ := range idsLeft {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			c.Logf("all instances terminated.")
			return
		}
	}
	c.Fatalf("%v INSTANCES LEFT RUNNING!!!", ids)
}

func (s *ServerTests) makeTestGroup(c *C, vpcId, name, descr string) ec2.SecurityGroup {
	// Clean it up if a previous test left it around.
	_, err := s.ec2.DeleteSecurityGroup(ec2.SecurityGroup{Name: name})
	if err != nil && errorCode(err) != "InvalidGroup.NotFound" {
		c.Fatalf("delete security group: %v", err)
	}

	resp, err := s.ec2.CreateSecurityGroup(vpcId, name, descr)
	c.Assert(err, IsNil)
	c.Assert(resp.Id, Matches, "sg-.+")
	c.Logf("created group %v", resp.SecurityGroup)
	return resp.SecurityGroup
}

func (s *ServerTests) TestIPPerms(c *C) {
	g0 := s.makeTestGroup(c, "", "goamz-test0", "ec2test group 0")
	g1 := s.makeTestGroup(c, "", "goamz-test1", "ec2test group 1")
	defer s.deleteGroups(c, []ec2.SecurityGroup{g0, g1})

	resp, err := s.ec2.SecurityGroups([]ec2.SecurityGroup{g0, g1}, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Groups, HasLen, 2)
	c.Assert(resp.Groups[0].IPPerms, HasLen, 0)
	c.Assert(resp.Groups[1].IPPerms, HasLen, 0)
	nameRegexp := fmt.Sprintf("(%s|%s)", regexp.QuoteMeta(g0.Name), regexp.QuoteMeta(g1.Name))
	for _, rgroup := range resp.Groups {
		c.Check(rgroup.Name, Matches, nameRegexp)
	}

	ownerId := resp.Groups[0].OwnerId

	// test some invalid parameters
	// TODO more
	_, err = s.ec2.AuthorizeSecurityGroup(g0, []ec2.IPPerm{{
		Protocol:  "tcp",
		FromPort:  0,
		ToPort:    1024,
		SourceIPs: []string{"invalid"},
	}})
	c.Assert(err, NotNil)
	c.Check(errorCode(err), Equals, "InvalidParameterValue")

	// Check that AuthorizeSecurityGroup adds the correct authorizations.
	_, err = s.ec2.AuthorizeSecurityGroup(g0, []ec2.IPPerm{{
		Protocol:  "tcp",
		FromPort:  2000,
		ToPort:    2001,
		SourceIPs: []string{"127.0.0.0/24"},
		SourceGroups: []ec2.UserSecurityGroup{{
			Name: g1.Name,
		}, {
			Id: g0.Id,
		}},
	}, {
		Protocol:  "tcp",
		FromPort:  2000,
		ToPort:    2001,
		SourceIPs: []string{"200.1.1.34/32"},
	}})
	c.Assert(err, IsNil)

	resp, err = s.ec2.SecurityGroups([]ec2.SecurityGroup{g0}, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Groups, HasLen, 1)
	c.Assert(resp.Groups[0].IPPerms, HasLen, 3)
	for _, ipperm := range resp.Groups[0].IPPerms {
		sourceIPs := ipperm.SourceIPs
		sourceGroups := ipperm.SourceGroups
		if len(sourceIPs) == 0 {
			// Only source groups should exist - one per IPPerm.
			c.Check(sourceGroups, HasLen, 1)
			c.Check(sourceIPs, IsNil)
			// Because groups can come in any order, check for either id.
			idRegexp := fmt.Sprintf("(%s|%s)", regexp.QuoteMeta(g0.Id), regexp.QuoteMeta(g1.Id))
			c.Check(sourceGroups[0].Id, Matches, idRegexp)
			c.Check(sourceGroups[0].Name, Equals, "")
			c.Check(sourceGroups[0].OwnerId, Equals, ownerId)
		} else if len(sourceGroups) == 0 {
			// Only source IPs should exist.
			c.Check(sourceGroups, IsNil)
			c.Check(sourceIPs, HasLen, 2)
			sort.Strings(sourceIPs)
			c.Check(sourceIPs, DeepEquals, []string{"127.0.0.0/24", "200.1.1.34/32"})
		}
		c.Check(ipperm.Protocol, Equals, "tcp")
		c.Check(ipperm.FromPort, Equals, 2000)
		c.Check(ipperm.ToPort, Equals, 2001)
	}

	// Check that we can't delete g1 (because g0 is using it)
	_, err = s.ec2.DeleteSecurityGroup(g1)
	c.Assert(err, NotNil)
	c.Check(errorCode(err), Equals, "DependencyViolation")

	_, err = s.ec2.RevokeSecurityGroup(g0, []ec2.IPPerm{{
		Protocol:     "tcp",
		FromPort:     2000,
		ToPort:       2001,
		SourceGroups: []ec2.UserSecurityGroup{{Id: g1.Id}},
		SourceIPs:    nil,
	}, {
		Protocol:     "tcp",
		FromPort:     2000,
		ToPort:       2001,
		SourceGroups: nil,
		SourceIPs:    []string{"200.1.1.34/32"},
	}})
	c.Assert(err, IsNil)

	resp, err = s.ec2.SecurityGroups([]ec2.SecurityGroup{g0}, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Groups, HasLen, 1)
	c.Assert(resp.Groups[0].IPPerms, HasLen, 2)
	for _, ipperm := range resp.Groups[0].IPPerms {
		sourceIPs := ipperm.SourceIPs
		sourceGroups := ipperm.SourceGroups
		if len(sourceIPs) == 0 {
			// Only source groups should exist - one per IPPerm.
			c.Check(sourceGroups, HasLen, 1)
			c.Check(sourceIPs, IsNil)
			c.Check(sourceGroups[0].Id, Matches, g0.Id)
			c.Check(sourceGroups[0].Name, Equals, "")
			c.Check(sourceGroups[0].OwnerId, Equals, ownerId)
		} else if len(sourceGroups) == 0 {
			// Only source IPs should exist.
			c.Check(sourceGroups, IsNil)
			c.Check(sourceIPs, HasLen, 1)
			c.Check(sourceIPs, DeepEquals, []string{"127.0.0.0/24"})
		}
		c.Check(ipperm.Protocol, Equals, "tcp")
		c.Check(ipperm.FromPort, Equals, 2000)
		c.Check(ipperm.ToPort, Equals, 2001)
	}

	// We should be able to delete g1 now because we've removed its only use.
	_, err = s.ec2.DeleteSecurityGroup(g1)
	c.Assert(err, IsNil)

	_, err = s.ec2.DeleteSecurityGroup(g0)
	c.Assert(err, IsNil)

	f := ec2.NewFilter()
	f.Add("group-id", g0.Id, g1.Id)
	resp, err = s.ec2.SecurityGroups(nil, f)
	c.Assert(err, IsNil)
	c.Assert(resp.Groups, HasLen, 0)
}

func (s *ServerTests) TestDuplicateIPPerm(c *C) {
	name := "goamz-test"
	descr := "goamz security group for tests"

	// Clean it up, if a previous test left it around and avoid leaving it around.
	s.ec2.DeleteSecurityGroup(ec2.SecurityGroup{Name: name})
	defer s.ec2.DeleteSecurityGroup(ec2.SecurityGroup{Name: name})

	resp1, err := s.ec2.CreateSecurityGroup("", name, descr)
	c.Assert(err, IsNil)
	c.Assert(resp1.Name, Equals, name)

	perms := []ec2.IPPerm{{
		Protocol:  "tcp",
		FromPort:  200,
		ToPort:    1024,
		SourceIPs: []string{"127.0.0.1/24"},
	}, {
		Protocol:  "tcp",
		FromPort:  0,
		ToPort:    100,
		SourceIPs: []string{"127.0.0.1/24"},
	}}

	_, err = s.ec2.AuthorizeSecurityGroup(ec2.SecurityGroup{Name: name}, perms[0:1])
	c.Assert(err, IsNil)

	_, err = s.ec2.AuthorizeSecurityGroup(ec2.SecurityGroup{Name: name}, perms[0:2])
	c.Assert(errorCode(err), Equals, "InvalidPermission.Duplicate")
}

type filterSpec struct {
	name   string
	values []string
}

func (s *ServerTests) TestInstanceFiltering(c *C) {
	vpcResp, err := s.ec2.CreateVPC("10.4.0.0/16", "")
	c.Assert(err, IsNil)
	vpcId := vpcResp.VPC.Id
	defer s.deleteVPCs(c, []string{vpcId})

	subResp := s.createSubnet(c, vpcId, "10.4.1.0/24", "")
	subId := subResp.Subnet.Id
	defer s.deleteSubnets(c, []string{subId})

	groupResp, err := s.ec2.CreateSecurityGroup(
		"", sessionName("testgroup1"),
		"testgroup one description",
	)
	c.Assert(err, IsNil)
	group1 := groupResp.SecurityGroup
	c.Assert(group1.Name, Not(Equals), "")
	c.Assert(group1.Id, Matches, "sg-.+")
	c.Logf("created group %v", group1)

	groupResp, err = s.ec2.CreateSecurityGroup(
		vpcId,
		sessionName("testgroup2"),
		"testgroup two description vpc",
	)
	c.Assert(err, IsNil)
	group2 := groupResp.SecurityGroup
	c.Assert(group2.Name, Not(Equals), "")
	c.Assert(group2.Id, Matches, "sg-.+")
	c.Logf("created group %v", group2)

	defer s.deleteGroups(c, []ec2.SecurityGroup{group1, group2})

	insts := make([]*ec2.Instance, 3)
	inst, err := s.ec2.RunInstances(&ec2.RunInstances{
		MinCount:       2,
		ImageId:        imageId,
		InstanceType:   "t1.micro",
		AvailZone:      defaultAvailZone,
		SecurityGroups: []ec2.SecurityGroup{group1},
	})
	c.Assert(err, IsNil)
	insts[0] = &inst.Instances[0]
	insts[1] = &inst.Instances[1]

	imageId2 := "ami-e358958a" // Natty server, i386, EBS store
	inst, err = s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:        imageId2,
		InstanceType:   "t1.micro",
		SubnetId:       subId,
		SecurityGroups: []ec2.SecurityGroup{group2},
	})
	c.Assert(err, IsNil)
	insts[2] = &inst.Instances[0]

	ids := func(indices ...int) (instIds []string) {
		for _, index := range indices {
			instIds = append(instIds, insts[index].InstanceId)
		}
		return
	}

	defer terminateInstances(c, s.ec2, ids(0, 1, 2))

	tests := []struct {
		about       string
		instanceIds []string     // instanceIds argument to Instances method.
		filters     []filterSpec // filters argument to Instances method.
		resultIds   []string     // set of instance ids of expected results.
		allowExtra  bool         // resultIds may be incomplete.
		err         string       // expected error.
	}{
		{
			about:      "check that Instances returns all instances",
			resultIds:  ids(0, 1, 2),
			allowExtra: true,
		}, {
			about:       "check that specifying two instance ids returns them",
			instanceIds: ids(0, 2),
			resultIds:   ids(0, 2),
		}, {
			about:       "check that specifying a non-existent instance id gives an error",
			instanceIds: append(ids(0), "i-deadbeef"),
			err:         `.*\(InvalidInstanceID\.NotFound\)`,
		}, {
			about: "check that a filter allowed both instances returns both of them",
			filters: []filterSpec{
				{"instance-id", ids(0, 2)},
			},
			resultIds: ids(0, 2),
		}, {
			about: "check that a filter allowing only one instance returns it",
			filters: []filterSpec{
				{"instance-id", ids(1)},
			},
			resultIds: ids(1),
		}, {
			about: "check that a filter allowing no instances returns none",
			filters: []filterSpec{
				{"instance-id", []string{"i-deadbeef12345"}},
			},
		}, {
			about: "check that filtering on group id with instance prefix works",
			filters: []filterSpec{
				{"instance.group-id", []string{group1.Id}},
			},
			resultIds: ids(0, 1),
		}, {
			about: "check that filtering on group name with instance prefix works",
			filters: []filterSpec{
				{"instance.group-name", []string{group1.Name}},
			},
			resultIds: ids(0, 1),
		}, {
			about: "check that filtering on image id works",
			filters: []filterSpec{
				{"image-id", []string{imageId}},
			},
			resultIds:  ids(0, 1),
			allowExtra: true,
		}, {
			about: "combination filters 1",
			filters: []filterSpec{
				{"image-id", []string{imageId, imageId2}},
				{"instance.group-name", []string{group1.Name}},
			},
			resultIds: ids(0, 1),
		}, {
			about: "combination filters 2",
			filters: []filterSpec{
				{"image-id", []string{imageId2}},
				{"instance.group-name", []string{group1.Name}},
			},
		}, {
			about: "VPC filters in combination",
			filters: []filterSpec{
				{"vpc-id", []string{vpcId}},
				{"subnet-id", []string{subId}},
			},
			resultIds: ids(2),
		},
	}
	for i, t := range tests {
		c.Logf("%d. %s", i, t.about)
		var f *ec2.Filter
		if t.filters != nil {
			f = ec2.NewFilter()
			for _, spec := range t.filters {
				f.Add(spec.name, spec.values...)
			}
		}
		c.Logf("\nusing filter: %v", f)
		c.Logf("\nexpecting results: %v", t.resultIds)
		resp, err := s.ec2.Instances(t.instanceIds, f)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err)
			continue
		}
		if !c.Check(err, IsNil) {
			continue
		}
		insts := make(map[string]*ec2.Instance)
		for _, r := range resp.Reservations {
			for j := range r.Instances {
				inst := &r.Instances[j]
				c.Check(insts[inst.InstanceId], IsNil, Commentf("duplicate instance id: %q", inst.InstanceId))
				insts[inst.InstanceId] = inst
			}
		}
		if !t.allowExtra {
			c.Check(insts, HasLen, len(t.resultIds), Commentf("expected %d instances got %#v", len(t.resultIds), insts))
		}
		for j, id := range t.resultIds {
			c.Check(insts[id], NotNil, Commentf("instance id %d (%q) not found; got %#v", j, id, insts))
		}
	}
}

func (s *AmazonServerSuite) TestRunInstancesVPC(c *C) {
	vpcResp, err := s.ec2.CreateVPC("10.6.0.0/16", "")
	c.Assert(err, IsNil)
	vpcId := vpcResp.VPC.Id
	defer s.deleteVPCs(c, []string{vpcId})

	subResp := s.createSubnet(c, vpcId, "10.6.1.0/24", "")
	subId := subResp.Subnet.Id
	defer s.deleteSubnets(c, []string{subId})

	groupResp, err := s.ec2.CreateSecurityGroup(
		vpcId,
		sessionName("testgroup1 vpc"),
		"testgroup description vpc",
	)
	c.Assert(err, IsNil)
	group := groupResp.SecurityGroup

	defer s.deleteGroups(c, []ec2.SecurityGroup{group})

	// Run a single instance with a new network interface.
	ips := []ec2.PrivateIP{
		{Address: "10.6.1.10", IsPrimary: true},
		{Address: "10.6.1.20", IsPrimary: false},
	}
	instResp, err := s.ec2.RunInstances(&ec2.RunInstances{
		MinCount:     1,
		ImageId:      imageId,
		InstanceType: "t1.micro",
		NetworkInterfaces: []ec2.RunNetworkInterface{{
			DeviceIndex:         0,
			SubnetId:            subId,
			PrivateIPs:          ips,
			SecurityGroupIds:    []string{group.Id},
			DeleteOnTermination: true,
		}},
	})
	c.Assert(err, IsNil)
	inst := &instResp.Instances[0]

	defer terminateInstances(c, s.ec2, []string{inst.InstanceId})

	// Now list the network interfaces and find ours.
	testAttempt := aws.AttemptStrategy{
		Total: 5 * time.Minute,
		Delay: 5 * time.Second,
	}
	f := ec2.NewFilter()
	f.Add("subnet-id", subId)
	var newNIC *ec2.NetworkInterface
	for a := testAttempt.Start(); a.Next(); {
		c.Logf("waiting for NIC to become available")
		listNICs, err := s.ec2.NetworkInterfaces(nil, f)
		if err != nil {
			c.Logf("retrying; NetworkInterfaces returned: %v", err)
			continue
		}
		for _, iface := range listNICs.Interfaces {
			c.Logf("found NIC %v", iface)
			if iface.Attachment.InstanceId == inst.InstanceId {
				c.Logf("instance %v new NIC appeared", inst.InstanceId)
				newNIC = &iface
				break
			}
		}
		if newNIC != nil {
			break
		}
	}
	if newNIC == nil {
		c.Fatalf("timeout while waiting for NIC to appear.")
	}
	c.Check(newNIC.Id, Matches, `^eni-[0-9a-f]+$`)
	c.Check(newNIC.SubnetId, Equals, subId)
	c.Check(newNIC.VPCId, Equals, vpcId)
	c.Check(newNIC.Status, Matches, `^(attaching|in-use)$`)
	c.Check(newNIC.PrivateIPAddress, Equals, ips[0].Address)
	c.Check(newNIC.PrivateIPs, DeepEquals, ips)
	c.Check(newNIC.Groups, HasLen, 1)
	c.Check(newNIC.Groups[0].Id, Equals, group.Id)
	c.Check(newNIC.Attachment.Status, Matches, `^(attaching|attached)$`)
	c.Check(newNIC.Attachment.DeviceIndex, Equals, 0)
	c.Check(newNIC.Attachment.DeleteOnTermination, Equals, true)
}

func (s *AmazonServerSuite) TestVolumeAttachments(c *C) {
	s.ServerTests.testVolumeAttachments(c)
}

func (s *AmazonServerSuite) TestModifyInstanceAttributeBlockDeviceMappings(c *C) {
	s.ServerTests.testModifyInstanceAttributeBlockDeviceMappings(c)
}

func (s *AmazonServerSuite) TestModifyInstanceAttributeSourceDestCheck(c *C) {
	s.ServerTests.testModifyInstanceAttributeSourceDestCheck(c)
}

func idsOnly(gs []ec2.SecurityGroup) []ec2.SecurityGroup {
	for i := range gs {
		gs[i].Name = ""
	}
	return gs
}

func namesOnly(gs []ec2.SecurityGroup) []ec2.SecurityGroup {
	for i := range gs {
		gs[i].Id = ""
	}
	return gs
}

func (s *ServerTests) TestGroupFiltering(c *C) {
	vpcResp, err := s.ec2.CreateVPC("10.5.0.0/16", "")
	c.Assert(err, IsNil)
	vpcId := vpcResp.VPC.Id
	defer s.deleteVPCs(c, []string{vpcId})

	subResp := s.createSubnet(c, vpcId, "10.5.1.0/24", "")
	subId := subResp.Subnet.Id
	defer s.deleteSubnets(c, []string{subId})

	g := make([]ec2.SecurityGroup, 5)
	for i := range g {
		var resp *ec2.CreateSecurityGroupResp
		gid := sessionName(fmt.Sprintf("testgroup%d", i))
		desc := fmt.Sprintf("testdescription%d", i)
		if i == 0 {
			// Create the first one as a VPC group.
			gid += " vpc"
			desc += " vpc"
			resp, err = s.ec2.CreateSecurityGroup(vpcId, gid, desc)
		} else {
			resp, err = s.ec2.CreateSecurityGroup("", gid, desc)
		}
		c.Assert(err, IsNil)
		g[i] = resp.SecurityGroup
		c.Logf("group %d: %v", i, g[i])
	}
	// Reorder the groups below, so that g[3] is first (some of the
	// reset depend on it, so they can't be deleted before g[3]). A
	// slight optimization for local live tests, so that we don't need
	// to wait 5s each time deleteGroups runs.
	defer s.deleteGroups(c, []ec2.SecurityGroup{g[3], g[0], g[1], g[2], g[4]})

	perms := [][]ec2.IPPerm{{{
		Protocol:  "tcp",
		FromPort:  100,
		ToPort:    200,
		SourceIPs: []string{"1.2.3.4/32"},
	}}, {{
		Protocol:     "tcp",
		FromPort:     200,
		ToPort:       300,
		SourceGroups: []ec2.UserSecurityGroup{{Id: g[2].Id}},
	}}, {{
		Protocol:     "udp",
		FromPort:     200,
		ToPort:       400,
		SourceGroups: []ec2.UserSecurityGroup{{Id: g[2].Id}},
	}}}
	for i, ps := range perms {
		_, err := s.ec2.AuthorizeSecurityGroup(g[i+1], ps)
		c.Assert(err, IsNil)
	}

	groups := func(indices ...int) (gs []ec2.SecurityGroup) {
		for _, index := range indices {
			gs = append(gs, g[index])
		}
		return
	}

	type groupTest struct {
		about      string
		groups     []ec2.SecurityGroup // groupIds argument to SecurityGroups method.
		filters    []filterSpec        // filters argument to SecurityGroups method.
		results    []ec2.SecurityGroup // set of expected result groups.
		allowExtra bool                // specified results may be incomplete.
		err        string              // expected error.
	}
	filterCheck := func(name, val string, gs []ec2.SecurityGroup) groupTest {
		return groupTest{
			about:      "filter check " + name,
			filters:    []filterSpec{{name, []string{val}}},
			results:    gs,
			allowExtra: true,
		}
	}
	tests := []groupTest{
		{
			about:      "check that SecurityGroups returns all groups",
			results:    groups(0, 1, 2, 3, 4),
			allowExtra: true,
		}, {
			about:   "check that specifying two group ids returns them",
			groups:  idsOnly(groups(0, 2)),
			results: groups(0, 2),
		}, {
			about:   "check that specifying names only works",
			groups:  namesOnly(groups(1, 2)),
			results: groups(1, 2),
		}, {
			about:  "check that specifying a non-existent group id gives an error",
			groups: append(groups(0), ec2.SecurityGroup{Id: "sg-eeeeeeee"}),
			err:    `.*\(InvalidGroup\.NotFound\)`,
		}, {
			about: "check that a filter allowed two groups returns both of them",
			filters: []filterSpec{
				{"group-id", []string{g[0].Id, g[2].Id}},
			},
			results: groups(0, 2),
		},
		{
			about:  "check that the previous filter works when specifying a list of ids",
			groups: groups(1, 2),
			filters: []filterSpec{
				{"group-id", []string{g[0].Id, g[2].Id}},
			},
			results: groups(2),
		}, {
			about: "check that a filter allowing no groups returns none",
			filters: []filterSpec{
				{"group-id", []string{"sg-eeeeeeee"}},
			},
		},
		filterCheck("description", "testdescription1", groups(1)),
		filterCheck("group-name", g[2].Name, groups(2)),
		filterCheck("group-id", g[2].Id, groups(2)),
		filterCheck("ip-permission.cidr", "1.2.3.4/32", groups(1)),
		filterCheck("ip-permission.group-id", g[2].Id, groups(2, 3)),
		filterCheck("ip-permission.protocol", "udp", groups(3)),
		filterCheck("ip-permission.from-port", "200", groups(2, 3)),
		filterCheck("ip-permission.to-port", "200", groups(1)),
		filterCheck("vpc-id", vpcId, groups(0)),
		// TODO owner-id
	}
	for i, t := range tests {
		c.Logf("%d. %s", i, t.about)
		var f *ec2.Filter
		if t.filters != nil {
			f = ec2.NewFilter()
			for _, spec := range t.filters {
				f.Add(spec.name, spec.values...)
			}
		}
		c.Logf("\nusing filter: %v", f)
		c.Logf("\nexpecting results: %v", t.results)
		resp, err := s.ec2.SecurityGroups(t.groups, f)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err)
			continue
		}
		if !c.Check(err, IsNil) {
			continue
		}
		groups := make(map[string]*ec2.SecurityGroup)
		for j := range resp.Groups {
			group := &resp.Groups[j].SecurityGroup
			c.Check(groups[group.Id], IsNil, Commentf("duplicate group id: %q", group.Id))
			groups[group.Id] = group
		}
		// If extra groups may be returned, eliminate all groups that
		// we did not create in this session apart from the default group.
		if t.allowExtra {
			namePat := regexp.MustCompile(sessionName("testgroup[0-9]"))
			for id, g := range groups {
				if !namePat.MatchString(g.Name) {
					delete(groups, id)
				}
			}
		}
		c.Check(groups, HasLen, len(t.results))
		for j, g := range t.results {
			rg := groups[g.Id]
			if c.Check(rg, NotNil, Commentf("group %d (%v) not found; got %#v", j, g, groups)) {
				c.Check(rg.Name, Equals, g.Name, Commentf("group %d (%v)", j, g))
			}
		}
	}
}

// deleteGroups ensures the given groups are deleted, by retrying
// until a timeout or all groups cannot be found anymore.
// This should be used to make sure tests leave no groups around.
func (s *ServerTests) deleteGroups(c *C, groups []ec2.SecurityGroup) {
	testAttempt := aws.AttemptStrategy{
		Total: 2 * time.Minute,
		Delay: 5 * time.Second,
	}
	for a := testAttempt.Start(); a.Next(); {
		deleted := 0
		c.Logf("deleting groups %v", groups)
		for _, group := range groups {
			_, err := s.ec2.DeleteSecurityGroup(group)
			if err == nil || errorCode(err) == "InvalidGroup.NotFound" {
				c.Logf("group %v deleted", group)
				deleted++
				continue
			}
			if err != nil {
				c.Logf("retrying; DeleteSecurityGroup returned: %v", err)
			}
		}
		if deleted == len(groups) {
			c.Logf("all groups deleted")
			return
		}
	}
	c.Fatalf("timeout while waiting %v groups to get deleted!", groups)
}

// testModifyInstanceAttributeBlockDeviceMappings is called by
// TestModifyInstanceAttributeBlockDeviceMappings in LocalServerSuite
// and AmazonServerSuite.
func (s *ServerTests) testModifyInstanceAttributeBlockDeviceMappings(c *C) {
	runInstancesResp, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
	})
	c.Assert(err, IsNil)
	c.Assert(runInstancesResp, NotNil)
	instanceId := runInstancesResp.Instances[0].InstanceId
	defer terminateInstances(c, s.ec2, []string{instanceId})

	createVolumeResp, err := s.ec2.CreateVolume(ec2.CreateVolume{
		AvailZone:  runInstancesResp.Instances[0].AvailZone,
		VolumeType: "standard",
		VolumeSize: 10,
	})
	c.Assert(err, IsNil)
	volumeId := createVolumeResp.Volume.Id
	defer deleteVolume(c, s.ec2, volumeId)
	// Terminate instance before volume, to ensure volume is detached.
	// We still need the previous terminateInstances in case CreateVolume
	// fails.
	defer terminateInstances(c, s.ec2, []string{instanceId})

	describeInstance := func() *ec2.Instance {
		resp, err := s.ec2.Instances([]string{instanceId}, nil)
		c.Assert(err, IsNil)
		c.Assert(resp.Reservations, HasLen, 1)
		c.Assert(resp.Reservations[0].Instances, HasLen, 1)
		return &resp.Reservations[0].Instances[0]
	}

	// Wait for the instance to be running, so we can attach the volumes.
	testAttempt := aws.AttemptStrategy{
		Total: 5 * time.Minute,
		Delay: 5 * time.Second,
	}
	var inst *ec2.Instance
	for a := testAttempt.Start(); a.Next(); {
		c.Logf("waiting for instance to be running")
		inst = describeInstance()
		if inst.State == ec2test.Running {
			break
		}
	}
	if inst == nil || inst.State != ec2test.Running {
		c.Fatalf("timeout while waiting for %v to be running", instanceId)
	}

	_, err = s.ec2.AttachVolume(volumeId, instanceId, "/dev/sdf1")
	c.Assert(err, IsNil)

	describeVolume := func() *ec2.Volume {
		resp, err := s.ec2.Volumes([]string{volumeId}, nil)
		c.Assert(err, IsNil)
		c.Assert(resp.Volumes, HasLen, 1)
		return &resp.Volumes[0]
	}

	// Wait for volume to be attached, so we can modify attributes.
	var attached bool
	for a := testAttempt.Start(); !attached && a.Next(); {
		c.Logf("waiting for volume to be attached")
		volume := describeVolume()
		if len(volume.Attachments) == 0 {
			continue
		}
		if volume.Attachments[0].Status == "attached" {
			attached = true
		}
	}
	if !attached {
		c.Fatalf("timeout while waiting for %v to be attached", volumeId)
	}

	volume := describeVolume()
	c.Assert(volume.Attachments, HasLen, 1)
	c.Assert(volume.Attachments[0].Device, Equals, "/dev/sdf1")
	c.Assert(volume.Attachments[0].DeleteOnTermination, Equals, false)

	_, err = s.ec2.ModifyInstanceAttribute(&ec2.ModifyInstanceAttribute{
		InstanceId: instanceId,
		BlockDeviceMappings: []ec2.InstanceBlockDeviceMapping{{
			DeviceName:          "/dev/sdf1",
			DeleteOnTermination: true,
		}},
	}, nil)
	c.Assert(err, IsNil)

	// Finally, wait for "delete on termination" to propagate, so we can
	// confirm that the ModifyInstanceAttribute does something.
	var deleteOnTermination bool
	for a := testAttempt.Start(); !deleteOnTermination && a.Next(); {
		c.Logf("waiting for DeleteOnTermination attribute to propagate")
		volume := describeVolume()
		c.Assert(volume.Attachments, HasLen, 1)
		deleteOnTermination = volume.Attachments[0].DeleteOnTermination
	}
	if !deleteOnTermination {
		c.Fatalf("timeout while waiting for delete-on-termination to be set")
	}
}

// testModifyInstanceAttributeSourceDestCheck is called by
// TestModifyInstanceAttributeSourceDestCheck in LocalServerSuite
// and AmazonServerSuite.
func (s *ServerTests) testModifyInstanceAttributeSourceDestCheck(c *C) {
	vpcResp, err := s.ec2.CreateVPC("10.6.0.0/16", "")
	c.Assert(err, IsNil)
	vpcId := vpcResp.VPC.Id
	defer s.deleteVPCs(c, []string{vpcId})

	subResp := s.createSubnet(c, vpcId, "10.6.1.0/24", "")
	subId := subResp.Subnet.Id
	defer s.deleteSubnets(c, []string{subId})

	groupResp, err := s.ec2.CreateSecurityGroup(
		vpcId,
		sessionName("testgroup1 vpc"),
		"testgroup description vpc",
	)
	c.Assert(err, IsNil)
	group := groupResp.SecurityGroup
	defer s.deleteGroups(c, []ec2.SecurityGroup{group})

	// Create a VPC instance; SourceDestCheck cannot be altered
	// for non-VPC instances.
	ips := []ec2.PrivateIP{
		{Address: "10.6.1.10", IsPrimary: true},
		{Address: "10.6.1.20", IsPrimary: false},
	}
	instResp, err := s.ec2.RunInstances(&ec2.RunInstances{
		MinCount:     1,
		ImageId:      imageId,
		InstanceType: "t1.micro",
		NetworkInterfaces: []ec2.RunNetworkInterface{{
			DeviceIndex:         0,
			SubnetId:            subId,
			PrivateIPs:          ips,
			SecurityGroupIds:    []string{group.Id},
			DeleteOnTermination: true,
		}},
	})
	c.Assert(err, IsNil)

	inst := &instResp.Instances[0]
	instanceId := inst.InstanceId
	defer terminateInstances(c, s.ec2, []string{instanceId})

	describeInstance := func() *ec2.Instance {
		resp, err := s.ec2.Instances([]string{instanceId}, nil)
		c.Assert(err, IsNil)
		c.Assert(resp.Reservations, HasLen, 1)
		c.Assert(resp.Reservations[0].Instances, HasLen, 1)
		return &resp.Reservations[0].Instances[0]
	}

	// Wait for the instance to be running, so we can check the default
	// attribute value for SourceDestCheck.
	testAttempt := aws.AttemptStrategy{
		Total: 5 * time.Minute,
		Delay: 5 * time.Second,
	}
	for a := testAttempt.Start(); a.Next(); {
		c.Logf("waiting for instance to be running")
		inst = describeInstance()
		if inst.State == ec2test.Running {
			break
		}
	}
	if inst == nil || inst.State != ec2test.Running {
		c.Fatalf("timeout while waiting for %v to be running", instanceId)
	}

	// SourceDestCheck is true by default.
	c.Assert(inst.SourceDestCheck, Equals, true)

	sourceDestCheck := false
	_, err = s.ec2.ModifyInstanceAttribute(&ec2.ModifyInstanceAttribute{
		InstanceId:      instanceId,
		SourceDestCheck: &sourceDestCheck,
	}, nil)
	c.Assert(err, IsNil)

	inst = describeInstance()
	c.Assert(inst.SourceDestCheck, Equals, false)
}

func (s *ServerTests) TestCreateTags(c *C) {
	list, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
		AvailZone:    defaultAvailZone,
	})
	c.Assert(err, IsNil)

	inst := list.Instances[0]
	c.Assert(inst, NotNil)

	id := inst.InstanceId
	defer terminateInstances(c, s.ec2, []string{id})

	c.Check(inst.Tags, HasLen, 0)
	for i := 0; i < 2; i++ {
		_, err = s.ec2.CreateTags([]string{id}, []ec2.Tag{
			{"tag1", ""},
			{"tag2", ""},
		})
		c.Check(err, IsNil)
	}

	_, err = s.ec2.CreateTags([]string{id}, []ec2.Tag{
		{"tag2", "2gat"},
	})

	resp, err := s.ec2.Instances([]string{id}, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Reservations, HasLen, 1)
	c.Assert(resp.Reservations[0].Instances, HasLen, 1)
	inst = resp.Reservations[0].Instances[0]

	tags := make(map[string]string)
	for _, tag := range inst.Tags {
		tags[tag.Key] = tag.Value
	}
	c.Check(tags, DeepEquals, map[string]string{
		"tag1": "",
		"tag2": "2gat",
	})
}

func (s *ServerTests) TestCreateTagsLimitExceeded(c *C) {
	list, err := s.ec2.RunInstances(&ec2.RunInstances{
		ImageId:      imageId,
		InstanceType: "t1.micro",
		AvailZone:    defaultAvailZone,
	})
	c.Assert(err, IsNil)
	inst := list.Instances[0]
	c.Assert(inst, NotNil)
	id := inst.InstanceId
	defer terminateInstances(c, s.ec2, []string{id})

	_, err = s.ec2.CreateTags([]string{id}, []ec2.Tag{
		{"tag1", ""},
		{"tag2", ""},
		{"tag3", ""},
		{"tag4", ""},
		{"tag5", ""},
		{"tag6", ""},
		{"tag7", ""},
		{"tag8", ""},
		{"tag9", ""},
		{"tag10", ""},
		{"tag11", ""},
	})
	c.Check(err, NotNil)
	c.Check(errorCode(err), Equals, "TagLimitExceeded")
}

func (s *ServerTests) TestCreateTagsInvalidId(c *C) {
	_, err := s.ec2.CreateTags([]string{"non-sense"}, []ec2.Tag{{"key", "value"}})
	c.Check(err, NotNil)
	c.Check(errorCode(err), Equals, "InvalidID")
}

// errorCode returns the code of the given error, assuming it's not
// nil and it's an instance of *ec2.Error. It returns an empty string
// otherwise.
func errorCode(err error) string {
	if err, _ := err.(*ec2.Error); err != nil {
		return err.Code
	}
	return ""
}
