//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//

package ec2test_test

import (
	"fmt"
	"reflect"
	"testing"

	. "gopkg.in/check.v1"

	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/ec2/ec2test"
)

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&S{})

type S struct {
	srv *ec2test.Server
	ec2 *ec2.EC2
}

func (s *S) SetUpSuite(c *C) {
	srv, err := ec2test.NewServer()
	c.Assert(err, IsNil)
	c.Assert(srv, NotNil)
	srv.SetCreateRootDisks(true)
	auth := aws.Auth{"abc", "123"}
	region := aws.Region{EC2Endpoint: srv.URL()}
	s.srv = srv
	s.ec2 = ec2.New(auth, region, aws.SignV2)
}

func (s *S) SetUpTest(c *C) {
	s.srv.Reset(false)
	_, err := s.srv.AddDefaultVPCAndSubnets()
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	s.srv.Quit()
}

func (s *S) TestAddVPC(c *C) {
	toAdd := ec2.VPC{
		Id:              "should-be-ignored",
		State:           "insane",
		CIDRBlock:       "0.1.2.0/24",
		DHCPOptionsId:   "also ignored",
		IsDefault:       true,
		InstanceTenancy: "foo",
	}
	added := s.srv.AddVPC(toAdd)
	c.Assert(added.Id, Matches, `^vpc-[0-9a-f]+$`)
	toAdd.Id = added.Id // only this differs
	c.Assert(added, DeepEquals, toAdd)

	resp, err := s.ec2.VPCs(nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.VPCs, HasLen, 2) // default and the one just added.
	c.Assert(resp.VPCs[0].IsDefault, Equals, true)
	c.Assert(resp.VPCs[1], DeepEquals, added)
}

func (s *S) TestUpdateVPC(c *C) {
	err := s.srv.UpdateVPC(ec2.VPC{})
	c.Assert(err, ErrorMatches, "missing VPC id")

	err = s.srv.UpdateVPC(ec2.VPC{Id: "missing"})
	c.Assert(err, ErrorMatches, `VPC "missing" not found`)
	toUpdate := ec2.VPC{
		Id:              "vpc-0", // the default VPC.
		State:           "insane",
		CIDRBlock:       "0.1.2.0/24",
		DHCPOptionsId:   "also ignored",
		IsDefault:       true,
		InstanceTenancy: "foo",
	}
	err = s.srv.UpdateVPC(toUpdate)
	c.Assert(err, IsNil)

	resp, err := s.ec2.VPCs(nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.VPCs, HasLen, 1)
	c.Assert(resp.VPCs[0], DeepEquals, toUpdate)
}

func (s *S) TestRemoveVPC(c *C) {
	// In addition to the default VPC's subnet add another VPC and
	// subnet and verify both subnets are returned by VPCs().
	vpc2 := s.srv.AddVPC(ec2.VPC{
		State:     "available",
		CIDRBlock: "10.20.0.0/16",
	})
	added, err := s.srv.AddSubnet(ec2.Subnet{
		VPCId:     vpc2.Id,
		CIDRBlock: "10.20.30.0/24",
		AvailZone: "us-east-1c", // default zone
	})
	c.Assert(err, IsNil)

	resp, err := s.ec2.Subnets(nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Subnets, HasLen, 2)
	for i := 0; i < len(resp.Subnets); i++ {
		c.Check(resp.Subnets[i].Id, Matches, "subnet-[01]") // default=0, added=1
		c.Check(resp.Subnets[i].VPCId, Matches, "vpc-[01]") // same as above
	}

	err = s.srv.RemoveVPC("")
	c.Assert(err, ErrorMatches, "missing VPC id")

	err = s.srv.RemoveVPC("missing")
	c.Assert(err, ErrorMatches, `VPC "missing" not found`)

	err = s.srv.RemoveVPC("vpc-0") // the default VPC added
	c.Assert(err, IsNil)

	// Make sure any subnet of the vpc-0 are gone.
	resp, err = s.ec2.Subnets(nil, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Subnets, HasLen, 1)
	c.Assert(resp.Subnets[0].Id, Equals, added.Id)
}

func (s *S) TestAddDefaultVPCAndSubnetsFailsWithoutZones(c *C) {
	assertNothingExists := func() {
		s.assertVPCsExist(c, false)
		s.assertInternetGatewaysExist(c, false)
		s.assertRouteTablesExist(c, false)
		s.assertZonesExist(c, false)
	}
	// Reset the test server and ensure there are neither VPCs nor
	// Zones set. Then start testing AddDefaultVPCAndSubnets().
	s.srv.Reset(true)
	assertNothingExists()

	// Ensure it fails when no zones are defined.
	defaultVPC, err := s.srv.AddDefaultVPCAndSubnets()
	c.Assert(err, ErrorMatches, "no AZs defined")
	c.Assert(defaultVPC.Id, Equals, "")
	assertNothingExists()
}

func (s *S) TestAddDefaultVPCAndSubnetsFailsWhenAddInternetGatewayFails(c *C) {
	s.resetAllButZones(c)

	restore := patchValue(
		ec2test.AddInternetGateway,
		func(*ec2test.Server, ec2.InternetGateway) (ec2.InternetGateway, error) {
			return ec2.InternetGateway{}, fmt.Errorf("igw boom!")
		})
	defer restore()

	// Ensure it fails when IGW cannot be added.
	defaultVPC, err := s.srv.AddDefaultVPCAndSubnets()
	c.Assert(err, ErrorMatches, "igw boom!")
	c.Assert(defaultVPC.Id, Equals, "")

	s.assertOnlyZonesRemain(c)
}

func (s *S) TestAddDefaultVPCAndSubnetsFailsWhenAddRouteTableFails(c *C) {
	s.resetAllButZones(c)

	restore := patchValue(
		ec2test.AddRouteTable,
		func(*ec2test.Server, ec2.RouteTable) (ec2.RouteTable, error) {
			return ec2.RouteTable{}, fmt.Errorf("rtb boom!")
		})
	defer restore()

	// Ensure it fails when the main route table cannot be added.
	defaultVPC, err := s.srv.AddDefaultVPCAndSubnets()
	c.Assert(err, ErrorMatches, "rtb boom!")
	c.Assert(defaultVPC.Id, Equals, "")

	s.assertOnlyZonesRemain(c)
}

func (s *S) TestAddDefaultVPCAndSubnetsFailsWhenAddSubnetFails(c *C) {
	s.resetAllButZones(c)

	restore := patchValue(
		ec2test.AddSubnet,
		func(*ec2test.Server, ec2.Subnet) (ec2.Subnet, error) {
			return ec2.Subnet{}, fmt.Errorf("subnet boom!")
		})
	defer restore()

	// Ensure it fails when subnets cannot be added.
	defaultVPC, err := s.srv.AddDefaultVPCAndSubnets()
	c.Assert(err, ErrorMatches, "subnet boom!")
	c.Assert(defaultVPC.Id, Equals, "")

	s.assertOnlyZonesRemain(c)
}

func (s *S) TestAddDefaultVPCAndSubnetsFailsWhenSetAccountAttributesFails(c *C) {
	s.resetAllButZones(c)

	restore := patchValue(
		ec2test.SetAccountAttributes,
		func(*ec2test.Server, map[string][]string) error {
			return fmt.Errorf("boom!")
		})
	defer restore()

	// Ensure it fails when default VPC attribute cannot be set.
	defaultVPC, err := s.srv.AddDefaultVPCAndSubnets()
	c.Assert(err, ErrorMatches, "boom!")
	c.Assert(defaultVPC.Id, Equals, "")

	s.assertOnlyZonesRemain(c)
}

func (s *S) TestVPCsIsDefaultFilter(c *C) {
	s.resetAllButZones(c)
	s.srv.AddVPC(ec2.VPC{
		State:           "insane",
		CIDRBlock:       "0.1.2.0/24",
		IsDefault:       false,
		InstanceTenancy: "foo",
	})
	vpc1 := s.srv.AddVPC(ec2.VPC{
		State:           "insane",
		CIDRBlock:       "0.2.4.0/24",
		IsDefault:       true,
		InstanceTenancy: "foo",
	})

	filter := ec2.NewFilter()
	filter.Add("isDefault", "true")
	resp, err := s.ec2.VPCs(nil, filter)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.VPCs, HasLen, 1)
	c.Assert(resp.VPCs[0].IsDefault, Equals, true)
	c.Assert(resp.VPCs[0].Id, Equals, vpc1.Id)
}

// patchValue sets the value pointed to by the given destination to
// the given value, and returns a function to restore it to its
// original value. The value must be assignable to the element type of
// the destination.
//
// NOTE: Taken from github.com/juju/testing/patch.go
func patchValue(dest, value interface{}) func() {
	destv := reflect.ValueOf(dest).Elem()
	oldv := reflect.New(destv.Type()).Elem()
	oldv.Set(destv)
	valuev := reflect.ValueOf(value)
	if !valuev.IsValid() {
		// This isn't quite right when the destination type is not
		// nilable, but it's better than the complex alternative.
		valuev = reflect.Zero(destv.Type())
	}
	destv.Set(valuev)
	return func() {
		destv.Set(oldv)
	}
}

func (s *S) assertVPCsExist(c *C, mustExist bool) {
	resp, err := s.ec2.VPCs(nil, nil)
	c.Assert(err, IsNil)
	if !mustExist {
		c.Assert(resp.VPCs, HasLen, 0)
	} else {
		c.Assert(resp.VPCs, Not(HasLen), 0)
	}
}

func (s *S) assertInternetGatewaysExist(c *C, mustExist bool) {
	resp, err := s.ec2.InternetGateways(nil, nil)
	c.Assert(err, IsNil)
	if !mustExist {
		c.Assert(resp.InternetGateways, HasLen, 0)
	} else {
		c.Assert(resp.InternetGateways, Not(HasLen), 0)
	}
}

func (s *S) assertZonesExist(c *C, mustExist bool) {
	resp, err := s.ec2.AvailabilityZones(nil)
	c.Assert(err, IsNil)
	if !mustExist {
		c.Assert(resp.Zones, HasLen, 0)
	} else {
		c.Assert(resp.Zones, Not(HasLen), 0)
	}
}

func (s *S) resetAllButZones(c *C) {
	// Reset the test server and ensure there are no VPCs, IGWs,
	// subnets, or route tables but there are AZs defined.
	s.srv.Reset(false)
	s.assertVPCsExist(c, false)
	s.assertInternetGatewaysExist(c, false)
	s.assertRouteTablesExist(c, false)
	s.assertZonesExist(c, true)
}

func (s *S) assertOnlyZonesRemain(c *C) {
	s.assertVPCsExist(c, false)
	s.assertInternetGatewaysExist(c, false)
	s.assertRouteTablesExist(c, false)
	s.assertZonesExist(c, true)
}
