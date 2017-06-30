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

// InternetGateway describes a VPC Internet Gateway.
//
// See http://goo.gl/OC1hcA for more details.
type InternetGateway struct {
	// Id is the id of the Internet Gateway (IGW).
	Id string `xml:"internetGatewayId"`

	// VPCId is the id of the VPC this IGW is attached to.
	// An IGW can be attached to only one VPC at a time.
	// Source: http://goo.gl/6ycqAx (Amazon VPC Limits).
	VPCId string `xml:"attachmentSet>item>vpcId"`

	// AttachmentState is the current state of the attachment.
	// Valid values: attaching | attached | detaching | detached
	AttachmentState string `xml:"attachmentSet>item>state"`

	// Tags holds any tags associated with the IGW.
	Tags []Tag `xml:"tagSet>item"`
}

// InternetGatewaysResp is the response to a InternetGateways request.
//
// See http://goo.gl/syjv2p for more details.
type InternetGatewaysResp struct {
	RequestId        string            `xml:"requestId"`
	InternetGateways []InternetGateway `xml:"internetGatewaySet>item"`
}

// InternetGateways describes one or more Internet Gateways (IGWs).
// Both parameters are optional, and if specified will limit the
// returned IGWs to the matching ids or filtering rules.
//
// See http://goo.gl/syjv2p for more details.
func (ec2 *EC2) InternetGateways(ids []string, filter *Filter) (resp *InternetGatewaysResp, err error) {
	params := makeParams("DescribeInternetGateways")
	for i, id := range ids {
		params["InternetGatewayId."+strconv.Itoa(i+1)] = id
	}
	filter.addParams(params)

	resp = &InternetGatewaysResp{}
	err = ec2.query(params, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
