//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//
// This file contains code handling AWS API around Internet Gateways.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net/http"

	"gopkg.in/amz.v3/ec2"
)

// AddInternetGateway inserts the given internet gateway in the test
// server, as if it was created using the simulated AWS API. The Id
// field of igw is ignored and replaced by the next igwId counter
// value, prefixed by "igw-". When set, the VPCId field must refer to
// a VPC the test server knows about. If VPCId is empty the IGW is
// considered not attached.
func (srv *Server) AddInternetGateway(igw ec2.InternetGateway) (ec2.InternetGateway, error) {
	zeroGateway := ec2.InternetGateway{}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if igw.VPCId != "" {
		if _, found := srv.vpcs[igw.VPCId]; !found {
			return zeroGateway, fmt.Errorf("VPC %q not found", igw.VPCId)
		}
	}
	added := &internetGateway{igw}
	added.Id = fmt.Sprintf("igw-%d", srv.igwId.next())
	srv.internetGateways[added.Id] = added
	return added.InternetGateway, nil
}

// UpdateInternetGateway updates the internet gateway info stored in
// the test server, matching the Id field of igw, replacing all the
// other values with igw's field values. Both the Id and VPCId fields
// (the latter when set) must refer to entities known by the test
// server, otherwise errors are returned. If VPCId is empty, this is
// treated as if the IGW is not attached to a VPC.
func (srv *Server) UpdateInternetGateway(igw ec2.InternetGateway) error {
	if igw.Id == "" {
		return fmt.Errorf("missing internet gateway id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	_, found := srv.internetGateways[igw.Id]
	if !found {
		return fmt.Errorf("internet gateway %q not found", igw.Id)
	}
	if igw.VPCId != "" {
		if _, found := srv.vpcs[igw.VPCId]; !found {
			return fmt.Errorf("VPC %q not found", igw.VPCId)
		}
	}
	srv.internetGateways[igw.Id] = &internetGateway{igw}
	return nil
}

// RemoveInternetGateway removes the internet gateway with the given
// igwId, stored in the test server. It's an error to try to remove an
// unknown or empty igwId.
func (srv *Server) RemoveInternetGateway(igwId string) error {
	if igwId == "" {
		return fmt.Errorf("missing internet gateway id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, found := srv.internetGateways[igwId]; found {
		delete(srv.internetGateways, igwId)
		return nil
	}
	return fmt.Errorf("internet gateway %q not found", igwId)
}

type internetGateway struct {
	ec2.InternetGateway
}

func (i *internetGateway) matchAttr(attr, value string) (ok bool, err error) {
	switch attr {
	case "internet-gateway-id":
		return i.Id == value, nil
	case "attachment.state":
		return i.AttachmentState == value, nil
	case "attachment.vpc-id":
		return i.VPCId == value, nil
	case "tag", "tag-key", "tag-value":
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (srv *Server) describeInternetGateways(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	idMap := parseIDs(req.Form, "InternetGatewayId.")
	f := newFilter(req.Form)
	var resp struct {
		XMLName xml.Name
		ec2.InternetGatewaysResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "DescribeInternetGatewaysResponse"}
	resp.RequestId = reqId
	for _, i := range srv.internetGateways {
		ok, err := f.ok(i)
		_, known := idMap[i.Id]
		if ok && (len(idMap) == 0 || known) {
			resp.InternetGateways = append(resp.InternetGateways, i.InternetGateway)
		} else if err != nil {
			fatalf(400, "InvalidParameterValue", "describe internet gateways: %v", err)
		}
	}
	return &resp
}
