//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//
// This file contains code handling AWS API around VPCs.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"

	"gopkg.in/amz.v3/ec2"
)

// AddRouteTable inserts the given route table in the test server, as
// if it was created using the simulated AWS API. The Id field of t is
// ignored and replaced by the next rtbId counter value, prefixed by
// "rtb-". When IsMain is true, the table becomes the main route table
// for its VPC.
//
// Any empty TableId field of an item in the Associations list will be
// set to the added table's Id automatically.
func (srv *Server) AddRouteTable(t ec2.RouteTable) (ec2.RouteTable, error) {
	if t.VPCId == "" {
		return ec2.RouteTable{}, fmt.Errorf("missing VPC id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, found := srv.vpcs[t.VPCId]; !found {
		return ec2.RouteTable{}, fmt.Errorf("VPC %q not found", t.VPCId)
	}
	added := &routeTable{t}
	added.Id = fmt.Sprintf("rtb-%d", srv.rtbId.next())
	for i, assoc := range added.Associations {
		assoc.Id = fmt.Sprintf("rtbassoc-%d", srv.rtbassocId.next())
		if assoc.TableId == "" {
			assoc.TableId = added.Id
		}
		added.Associations[i] = assoc
	}
	srv.routeTables[added.Id] = added
	return added.RouteTable, nil
}

// UpdateRouteTable updates the route table info stored in the test
// server, matching the Id field of t, replacing all the other values
// with t's field values. It's an error to try to update a table with
// unknown or empty Id.
func (srv *Server) UpdateRouteTable(t ec2.RouteTable) error {
	if t.Id == "" {
		return fmt.Errorf("missing route table id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	rtb, found := srv.routeTables[t.Id]
	if !found {
		return fmt.Errorf("route table %q not found", t.Id)
	}
	rtb.VPCId = t.VPCId
	rtb.Associations = make([]ec2.RouteTableAssociation, len(t.Associations))
	copy(rtb.Associations, t.Associations)
	rtb.Routes = make([]ec2.Route, len(t.Routes))
	copy(rtb.Routes, t.Routes)
	rtb.PropagatingVGWIds = make([]string, len(t.PropagatingVGWIds))
	copy(rtb.PropagatingVGWIds, t.PropagatingVGWIds)
	srv.routeTables[t.Id] = rtb
	return nil
}

// RemoveRouteTable removes an route table with the given rtbId from
// the test server. It's an error to try to remove an unknown or empty
// rtbId.
func (srv *Server) RemoveRouteTable(rtbId string) error {
	if rtbId == "" {
		return fmt.Errorf("missing route table id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, found := srv.routeTables[rtbId]; found {
		delete(srv.routeTables, rtbId)
		return nil
	}
	return fmt.Errorf("route table %q not found", rtbId)
}

type routeTable struct {
	ec2.RouteTable
}

func (t *routeTable) matchAttr(attr, value string) (ok bool, err error) {
	filterByAssociation := func(check func(assoc ec2.RouteTableAssociation) bool) (bool, error) {
		for _, assoc := range t.Associations {
			if check(assoc) {
				return true, nil
			}
		}
		return false, nil
	}
	filterByRoute := func(check func(route ec2.Route) bool) (bool, error) {
		for _, route := range t.Routes {
			if check(route) {
				return true, nil
			}
		}
		return false, nil
	}

	switch attr {
	case "route-table-id":
		return t.Id == value, nil
	case "vpc-id":
		return t.VPCId == value, nil
	case "route.destination-cidr-block":
		return filterByRoute(func(r ec2.Route) bool {
			return r.DestinationCIDRBlock == value
		})
	case "route.gateway-id":
		return filterByRoute(func(r ec2.Route) bool {
			return r.GatewayId == value
		})
	case "route.instance-id":
		return filterByRoute(func(r ec2.Route) bool {
			return r.InstanceId == value
		})
	case "route.state":
		return filterByRoute(func(r ec2.Route) bool {
			return r.State == value
		})
	case "association.main":
		val, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("bad flag %q: %s", attr, value)
		}
		return filterByAssociation(func(a ec2.RouteTableAssociation) bool {
			return a.IsMain == val
		})
	case "association.subnet-id":
		return filterByAssociation(func(a ec2.RouteTableAssociation) bool {
			return a.SubnetId == value
		})
	case "association.route-table-id":
		return filterByAssociation(func(a ec2.RouteTableAssociation) bool {
			return a.TableId == value
		})
	case "association.route-table-association-id":
		return filterByAssociation(func(a ec2.RouteTableAssociation) bool {
			return a.Id == value
		})
	case "tag", "tag-key", "tag-value", "route.origin",
		"route.destination-prefix-list-id", "route.vpc-peering-connection-id":
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (srv *Server) describeRouteTables(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	idMap := parseIDs(req.Form, "RouteTableId.")
	f := newFilter(req.Form)
	var resp struct {
		XMLName xml.Name
		ec2.RouteTablesResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "DescribeRouteTablesResponse"}
	resp.RequestId = reqId
	for _, t := range srv.routeTables {
		ok, err := f.ok(t)
		_, known := idMap[t.Id]
		if ok && (len(idMap) == 0 || known) {
			resp.Tables = append(resp.Tables, t.RouteTable)
		} else if err != nil {
			fatalf(400, "InvalidParameterValue", "describe route tables: %v", err)
		}
	}
	return &resp
}
