//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//
// This file contains code handling AWS API around VPCs.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net/http"

	"gopkg.in/amz.v3/ec2"
)

// AddVPC inserts the given VPC in the test server, as if it was
// created using the simulated AWS API. The Id field of v is ignored
// and replaced by the next vpcId counter value, prefixed by "vpc-".
// When IsDefault is true, the VPC becomes the default VPC for the
// simulated region.
func (srv *Server) AddVPC(v ec2.VPC) ec2.VPC {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	added := &vpc{v}
	added.Id = fmt.Sprintf("vpc-%d", srv.vpcId.next())
	srv.vpcs[added.Id] = added
	return added.VPC
}

// UpdateVPC updates the VPC info stored in the test server, matching
// the Id field of v, replacing all the other values with v's field
// values. It's an error to try to update a VPC with unknown or empty
// Id.
func (srv *Server) UpdateVPC(v ec2.VPC) error {
	if v.Id == "" {
		return fmt.Errorf("missing VPC id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	vpc, found := srv.vpcs[v.Id]
	if !found {
		return fmt.Errorf("VPC %q not found", v.Id)
	}
	vpc.CIDRBlock = v.CIDRBlock
	vpc.DHCPOptionsId = v.DHCPOptionsId
	vpc.InstanceTenancy = v.InstanceTenancy
	vpc.Tags = append([]ec2.Tag{}, v.Tags...)
	vpc.IsDefault = v.IsDefault
	vpc.State = v.State
	srv.vpcs[v.Id] = vpc
	return nil
}

// RemoveVPC removes an existing VPC with the given vpcId from the
// test server. It's an error to try to remove an unknown or empty
// vpcId.
//
// NOTE: Removing a VPC will remove all of its subnets.
func (srv *Server) RemoveVPC(vpcId string) error {
	if vpcId == "" {
		return fmt.Errorf("missing VPC id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, found := srv.vpcs[vpcId]; found {
		delete(srv.vpcs, vpcId)
		remainingSubnets := make(map[string]*subnet)
		for _, sub := range srv.subnets {
			if sub.VPCId != vpcId {
				remainingSubnets[sub.Id] = sub
			}
		}
		srv.subnets = remainingSubnets
		return nil
	}
	return fmt.Errorf("VPC %q not found", vpcId)
}

// The following variables make writing tests easier.
var (
	addInternetGateway   = (*Server).AddInternetGateway
	addSubnet            = (*Server).AddSubnet
	addRouteTable        = (*Server).AddRouteTable
	setAccountAttributes = (*Server).SetAccountAttributes
)

// AddDefaultVPCAndSubnets makes it easy to simulate a default VPC is
// present in the test server. Calling this method is more or less
// an equivalent of calling the following methods as described:
//
// 1. AddVPC(), using 10.10.0.0/16 as CIDR and sane defaults.
// 2. AddInternetGateway(), attached to the default VPC.
// 3. AddRouteTable(), attached to the default VPC, with sane defaults
// and using the IGW above as default route.
// 4. AddSubnet(), once per defined AZ, with 10.10.X.0/24 CIDR (X
// is a zero-based index). Each subnet has both DefaultForAZ and
// MapPublicIPOnLaunch attributes set.
// 5. SetAccountAttributes(), with "supported-platforms" set to "EC2",
// "VPC"; and "default-vpc" set to the added default VPC.
//
// NOTE: If no AZs are set on the test server, this method fails.
func (srv *Server) AddDefaultVPCAndSubnets() (defaultVPC ec2.VPC, err error) {
	zeroVPC := ec2.VPC{}
	var igw ec2.InternetGateway
	var rtbMain ec2.RouteTable

	defer func() {
		// Cleanup all partially added items on error.
		if err != nil && defaultVPC.Id != "" {
			srv.RemoveVPC(defaultVPC.Id)
			if rtbMain.Id != "" {
				srv.RemoveRouteTable(rtbMain.Id)
			}
			if igw.Id != "" {
				srv.RemoveInternetGateway(igw.Id)
			}
			srv.SetAccountAttributes(map[string][]string{})
			defaultVPC.Id = "" // it's gone anyway.
		}
	}()

	if len(srv.zones) == 0 {
		return zeroVPC, fmt.Errorf("no AZs defined")
	}
	defaultVPC = srv.AddVPC(ec2.VPC{
		State:           "available",
		CIDRBlock:       "10.10.0.0/16",
		DHCPOptionsId:   fmt.Sprintf("dopt-%d", srv.dhcpOptsId.next()),
		InstanceTenancy: "default",
		IsDefault:       true,
	})
	zeroVPC.Id = defaultVPC.Id // zeroed again in the deferred
	igw, err = addInternetGateway(srv, ec2.InternetGateway{
		VPCId:           defaultVPC.Id,
		AttachmentState: "available",
	})
	if err != nil {
		return zeroVPC, err
	}
	rtbMain, err = addRouteTable(srv, ec2.RouteTable{
		VPCId: defaultVPC.Id,
		Associations: []ec2.RouteTableAssociation{{
			IsMain: true,
		}},
		Routes: []ec2.Route{{
			DestinationCIDRBlock: defaultVPC.CIDRBlock, // default VPC internal traffic
			GatewayId:            "local",
			State:                "active",
		}, {
			DestinationCIDRBlock: "0.0.0.0/0", // default VPC default egress route.
			GatewayId:            igw.Id,
			State:                "active",
		}},
	})
	if err != nil {
		return zeroVPC, err
	}
	subnetIndex := 0
	for zone, _ := range srv.zones {
		cidrBlock := fmt.Sprintf("10.10.%d.0/24", subnetIndex)
		availIPs, _ := srv.calcSubnetAvailIPs(cidrBlock)
		_, err = addSubnet(srv, ec2.Subnet{
			VPCId:            defaultVPC.Id,
			State:            "available",
			CIDRBlock:        cidrBlock,
			AvailZone:        zone,
			AvailableIPCount: availIPs,
			DefaultForAZ:     true,
		})
		if err != nil {
			return zeroVPC, err
		}
		subnetIndex++
	}
	err = setAccountAttributes(srv, map[string][]string{
		"supported-platforms": []string{"EC2", "VPC"},
		"default-vpc":         []string{defaultVPC.Id},
	})
	if err != nil {
		return zeroVPC, err
	}
	return defaultVPC, nil
}

type vpc struct {
	ec2.VPC
}

func (v *vpc) matchAttr(attr, value string) (ok bool, err error) {
	switch attr {
	case "cidr":
		return v.CIDRBlock == value, nil
	case "state":
		return v.State == value, nil
	case "vpc-id":
		return v.Id == value, nil
	case "isDefault":
		return v.IsDefault == (value == "true"), nil
	case "tag", "tag-key", "tag-value", "dhcp-options-id":
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (srv *Server) createVpc(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	cidrBlock := parseCidr(req.Form.Get("CidrBlock"))
	tenancy := req.Form.Get("InstanceTenancy")
	if tenancy == "" {
		tenancy = "default"
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	v := &vpc{ec2.VPC{
		Id:              fmt.Sprintf("vpc-%d", srv.vpcId.next()),
		State:           "available",
		CIDRBlock:       cidrBlock,
		DHCPOptionsId:   fmt.Sprintf("dopt-%d", srv.dhcpOptsId.next()),
		InstanceTenancy: tenancy,
	}}
	srv.vpcs[v.Id] = v
	var resp struct {
		XMLName xml.Name
		ec2.CreateVPCResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "CreateVpcResponse"}
	resp.RequestId = reqId
	resp.VPC = v.VPC
	return resp
}

func (srv *Server) deleteVpc(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	v := srv.vpc(req.Form.Get("VpcId"))
	srv.mu.Lock()
	defer srv.mu.Unlock()

	delete(srv.vpcs, v.Id)
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "DeleteVpcResponse"},
		RequestId: reqId,
		Return:    true,
	}
}

func (srv *Server) describeVpcs(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	idMap := parseIDs(req.Form, "VpcId.")
	f := newFilter(req.Form)
	var resp struct {
		XMLName xml.Name
		ec2.VPCsResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "DescribeVpcsResponse"}
	resp.RequestId = reqId
	for _, v := range srv.vpcs {
		ok, err := f.ok(v)
		_, known := idMap[v.Id]
		if ok && (len(idMap) == 0 || known) {
			resp.VPCs = append(resp.VPCs, v.VPC)
		} else if err != nil {
			fatalf(400, "InvalidParameterValue", "describe VPCs: %v", err)
		}
	}
	return &resp
}

func (srv *Server) vpc(id string) *vpc {
	if id == "" {
		fatalf(400, "MissingParameter", "missing vpcId")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	v, found := srv.vpcs[id]
	if !found {
		fatalf(400, "InvalidVpcID.NotFound", "VPC %s not found", id)
	}
	return v
}
