//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//

package ec2

import (
	"strconv"
)

// RouteTableAssociation describes an association between a route table and a subnet.
//
// See http://goo.gl/ZgrG5j for more details.
type RouteTableAssociation struct {
	// Id is the route table association id.
	Id string `xml:"routeTableAssociationId"`

	// TableId is the route table id for this association.
	TableId string `xml:"routeTableId"`

	// SubnetId is the subnet id (only when explicitly associated).
	SubnetId string `xml:"subnetId"`

	// IsMain indicates whether this is the main route table.
	IsMain bool `xml:"main"`
}

// Route describes a single route in a route table.
//
// See http://goo.gl/GlW6ii for more details.
type Route struct {
	// DestinationCIDRBlock is used for destination matching.
	DestinationCIDRBlock string `xml:"destinationCidrBlock"`

	// DestinationPrefixListId is the prefix of the AWS service.
	DestinationPrefixListId string `xml:"destinationPrefixListId"`

	// GatewayId is the id of an Internet Gateway attached to the VPC.
	GatewayId string `xml:"gatewayId"`

	// InstanceId is the id of a NAT instance in the VPC.
	InstanceId string `xml:"instanceId"`

	// InstanceOwnerId is the AWS account id of the NAT instance
	// owner.
	InstanceOwnerId string `xml:"instanceOwnerId"`

	// InterfaceId is the id of the used Network Interface.
	InterfaceId string `xml:"networkInterfaceId"`

	// Origin describes how the route was created.
	// Values: CreateRouteTable | CreateRoute | EnableVgwRoutePropagation
	Origin string `xml:"origin"`

	// State is the state of the route. The blackhole state indicates
	// the route target isn't available (e.g. IGW isn't attached or
	// NAT instance not found).
	State string `xml:"state"`

	// VPCPeeringConnectionId is the id of the VPC peering connection.
	VPCPeeringConnectionId string `xml:"vpcPeeringConnectionId"`
}

// RouteTable describes a VPC route table.
//
// See http://goo.gl/h0bwYw  for more details.
type RouteTable struct {
	// Id is the id of the Internet Gateway (IGW).
	Id string `xml:"routeTableId"`

	// VPCId is the id of the VPC this route table is attached to.
	VPCId string `xml:"vpcId"`

	// Associations holds associations between the route table and one
	// or more subnets.
	Associations []RouteTableAssociation `xml:"associationSet>item"`

	// Routes holds all route table routes.
	Routes []Route `xml:"routeSet>item"`

	// Tags holds any tags associated with the IGW.
	Tags []Tag `xml:"tagSet>item"`

	// PropagatingVGWIds holds a list of ids of propagating virtual
	// private gateways.
	PropagatingVGWIds []string `xml:"propagatingVgws>gatewayId"`
}

// RouteTablesResp is the response to a RouteTables request.
//
// See http://goo.gl/1JZIfO for more details.
type RouteTablesResp struct {
	RequestId string       `xml:"requestId"`
	Tables    []RouteTable `xml:"routeTableSet>item"`
}

// RouteTables describes one or more route tables.
// Both parameters are optional, and if specified will limit the
// returned tables to the matching ids or filtering rules.
//
// See http://goo.gl/1JZIfO for more details.
func (ec2 *EC2) RouteTables(ids []string, filter *Filter) (resp *RouteTablesResp, err error) {
	params := makeParams("DescribeRouteTables")
	for i, id := range ids {
		params["RouteTableId."+strconv.Itoa(i+1)] = id
	}
	filter.addParams(params)

	resp = &RouteTablesResp{}
	err = ec2.query(params, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
