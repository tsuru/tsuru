//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//

package ec2_test

import (
	. "gopkg.in/check.v1"

	"gopkg.in/amz.v3/ec2"
)

// Route Table tests with example responses

func (s *S) TestRouteTablesExample(c *C) {
	testServer.Response(200, nil, DescribeRouteTablesExample)

	ids := []string{"rtb-13ad487a", "rtb-f9ad4890"}
	resp, err := s.ec2.RouteTables(ids, nil)
	req := testServer.WaitRequest()

	c.Assert(req.Form["Action"], DeepEquals, []string{"DescribeRouteTables"})
	c.Assert(req.Form["RouteTableId.1"], DeepEquals, []string{ids[0]})
	c.Assert(req.Form["RouteTableId.2"], DeepEquals, []string{ids[1]})

	c.Assert(err, IsNil)
	c.Assert(resp.RequestId, Equals, "6f570b0b-9c18-4b07-bdec-73740dcf861a")
	c.Assert(resp.Tables, HasLen, 2)

	table := resp.Tables[0]
	c.Check(table.Id, Equals, "rtb-13ad487a")
	c.Check(table.VPCId, Equals, "vpc-11ad4878")
	c.Check(table.Routes, HasLen, 1)
	route := table.Routes[0]
	c.Check(route.DestinationCIDRBlock, Equals, "10.0.0.0/22")
	c.Check(route.GatewayId, Equals, "local")
	c.Check(route.State, Equals, "active")
	c.Check(route.Origin, Equals, "CreateRouteTable")
	c.Check(table.Associations, HasLen, 1)
	assoc := table.Associations[0]
	c.Check(assoc.Id, Equals, "rtbassoc-12ad487b")
	c.Check(assoc.TableId, Equals, "rtb-13ad487a")
	c.Check(assoc.IsMain, Equals, true)
	c.Check(table.Tags, HasLen, 0)

	table = resp.Tables[1]
	c.Check(table.Id, Equals, "rtb-f9ad4890")
	c.Check(table.VPCId, Equals, "vpc-11ad4878")
	c.Check(table.Routes, HasLen, 2)
	route = table.Routes[0]
	c.Check(route.DestinationCIDRBlock, Equals, "10.0.0.0/22")
	c.Check(route.GatewayId, Equals, "local")
	c.Check(route.State, Equals, "active")
	c.Check(route.Origin, Equals, "CreateRouteTable")
	route = table.Routes[1]
	c.Check(route.DestinationCIDRBlock, Equals, "0.0.0.0/0")
	c.Check(route.GatewayId, Equals, "igw-eaad4883")
	c.Check(route.State, Equals, "active")
	c.Check(table.Associations, HasLen, 1)
	assoc = table.Associations[0]
	c.Check(assoc.Id, Equals, "rtbassoc-faad4893")
	c.Check(assoc.TableId, Equals, "rtb-f9ad4890")
	c.Check(assoc.SubnetId, Equals, "subnet-15ad487c")
	c.Check(table.Tags, HasLen, 0)
}

// Route Table tests that run either against the local test
// server or live EC2 servers.

func (s *ServerTests) TestDefaultVPCRouteTables(c *C) {
	defaultVPCId, _ := s.getDefaultVPCIdAndSubnets(c)

	filter := ec2.NewFilter()
	filter.Add("vpc-id", defaultVPCId)
	// Look it up by VPC id filter.
	resp1, err := s.ec2.RouteTables(nil, filter)
	c.Assert(err, IsNil)
	// There should be at least one for the default VPC.
	c.Assert(resp1.Tables, Not(HasLen), 0)
	tableIds := make([]string, len(resp1.Tables))
	for i, table := range resp1.Tables {
		c.Check(table.VPCId, Equals, defaultVPCId)
		c.Check(table.Id, Matches, `^rtb-[0-9a-f]+$`)
		tableIds[i] = table.Id
		for _, route := range table.Routes {
			c.Check(route.GatewayId, Matches, `^(local|igw-[0-9a-f]+)$`)
			if route.GatewayId != "local" {
				// Default VPC always has IGW attached.
				c.Check(route.DestinationCIDRBlock, Equals, "0.0.0.0/0")
				c.Check(route.State, Equals, "active")
				c.Check(route.Origin, Matches, "(|CreateRouteTable|CreateRoute)")
			}
		}
		for _, assoc := range table.Associations {
			c.Check(assoc.Id, Matches, `^rtbassoc-[0-9a-f]+$`)
			c.Check(assoc.TableId, Equals, table.Id)
			if !assoc.IsMain {
				// Only the main route table is implictly associated
				// with all default VPC subnets, the other tables must
				// be associated with a subnet explicitly.
				c.Check(assoc.SubnetId, Matches, `^subnet-[0-9a-f]+$`)
			}
		}
	}

	// Look it up by ids and no filter.
	resp2, err := s.ec2.RouteTables(tableIds, nil)
	c.Assert(err, IsNil)
	resp2.RequestId = resp1.RequestId // the only difference.
	c.Assert(resp1, DeepEquals, resp2)
}

func (s *ServerTests) TestDefaultVPCRouteTablesAllFilters(c *C) {
}
