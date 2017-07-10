//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//
// This file contains code handling AWS API around Security Groups and
// permissions.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/amz.v3/ec2"
)

// securityGroup holds a simulated ec2 security group.
// Instances of securityGroup should only be created through
// Server.createSecurityGroup to ensure that groups can be
// compared by pointer value.
type securityGroup struct {
	id          string
	name        string
	description string
	vpcId       string

	perms map[permKey]bool
	tags  []ec2.Tag
}

// securityGroupInfo is almost the same as ec2.SecurityGroupInfo, but
// properly serializes IPPerms.SourceIPs. See the ipPerm type for more
// info.
type securityGroupInfo struct {
	ec2.SecurityGroup
	VPCId       string   `xml:"vpcId"`
	OwnerId     string   `xml:"ownerId"`
	Description string   `xml:"groupDescription"`
	IPPerms     []ipPerm `xml:"ipPermissions>item"`
}

// permKey represents permission for a given security
// group or IP address (but not both) to access a given range of
// ports. Equality of permKeys is used in the implementation of
// permission sets, relying on the uniqueness of securityGroup
// instances.
type permKey struct {
	protocol string
	fromPort int
	toPort   int
	group    *securityGroup
	ipAddr   string
}

// ipPerm describes a security group ingress permission rule the way
// EC2 API defines it. The only difference between this and ec2.IPPerm
// is the way SourceIPs get serialized, i.e.
// <item><cidrIp>1.2.3.4/5</cidrIp></item><item><cidrIp>5.4.3.2/1</cidrIp></item>,
// as EC2 does, rather than
// <item><cidrIp>1.2.3.4/5</cidrIp><cidrIp>5.4.3.2/1</cidrIp></item>,
// as ec2.IPPerm defines. Due to the flexibility of Go's xml package
// both forms above can be deserialized to the same
// []string{"1.2.3.4/5", "5.4.3.2/1"} value.
type ipPerm struct {
	Protocol     string                  `xml:"ipProtocol"`
	FromPort     int                     `xml:"fromPort"`
	ToPort       int                     `xml:"toPort"`
	SourceIPs    []sourceIP              `xml:"ipRanges>item"`
	SourceGroups []ec2.UserSecurityGroup `xml:"groups>item"`
}

type sourceIP struct {
	CIDRIP string `xml:"cidrIp"`
}

// Patterns used to match a security group, CIDR, or owner ID.
var (
	secGroupPat = regexp.MustCompile(`^sg-[a-f0-9]+$`)
	cidrIpPat   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/([0-9]+)$`)
	ownerIdPat  = regexp.MustCompile(`^[0-9]+$`)
)

func (g *securityGroup) ec2SecurityGroup() ec2.SecurityGroup {
	return ec2.SecurityGroup{
		Name: g.name,
		Id:   g.id,
	}
}

func (g *securityGroup) matchAttr(attr, value string) (ok bool, err error) {
	switch attr {
	case "description":
		return g.description == value, nil
	case "group-id":
		return g.id == value, nil
	case "group-name":
		return g.name == value, nil
	case "ip-permission.cidr":
		return g.hasPerm(func(k permKey) bool { return k.ipAddr == value }), nil
	case "ip-permission.group-name":
		return g.hasPerm(func(k permKey) bool {
			return k.group != nil && k.group.name == value
		}), nil
	case "ip-permission.group-id":
		return g.hasPerm(func(k permKey) bool {
			return k.group != nil && k.group.id == value
		}), nil
	case "ip-permission.from-port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return g.hasPerm(func(k permKey) bool { return k.fromPort == port }), nil
	case "ip-permission.to-port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return g.hasPerm(func(k permKey) bool { return k.toPort == port }), nil
	case "ip-permission.protocol":
		return g.hasPerm(func(k permKey) bool { return k.protocol == value }), nil
	case "owner-id":
		return value == ownerId, nil
	case "vpc-id":
		return g.vpcId == value, nil
	}
	if strings.HasPrefix(attr, "tag:") {
		key := attr[len("tag:"):]
		return matchTag(g.tags, key, value), nil
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (g *securityGroup) hasPerm(test func(k permKey) bool) bool {
	for k := range g.perms {
		if test(k) {
			return true
		}
	}
	return false
}

// ec2Perms returns the list of EC2 permissions granted
// to g. It groups permissions by port range and protocol.
func (g *securityGroup) ec2Perms() (perms []ipPerm) {
	// The grouping is held in result. We use permKey for convenience,
	// (ensuring that the ipAddr of each key is zero). For each
	// protocol/port range combination, we build up the permission set
	// in the associated value.
	result := make(map[permKey]*ipPerm)
	for k := range g.perms {
		groupKey := k
		groupKey.ipAddr = ""

		ec2p := result[groupKey]
		if ec2p == nil {
			ec2p = &ipPerm{
				Protocol: k.protocol,
				FromPort: k.fromPort,
				ToPort:   k.toPort,
			}
		}
		if k.group != nil {
			ec2p.SourceGroups = append(ec2p.SourceGroups,
				ec2.UserSecurityGroup{
					Id:      k.group.id,
					OwnerId: ownerId,
				})
		} else if k.ipAddr != "" {
			ec2p.SourceIPs = append(ec2p.SourceIPs, sourceIP{k.ipAddr})
		}
		result[groupKey] = ec2p
	}
	for _, ec2p := range result {
		perms = append(perms, *ec2p)
	}
	return
}

// formToGroups parses a set of SecurityGroup form values
// as found in a RunInstances request, and returns the resulting
// slice of security groups.
// It calls fatalf if a group is not found.
func (srv *Server) formToGroups(form url.Values) []*securityGroup {
	var groups []*securityGroup
	for name, values := range form {
		switch {
		case strings.HasPrefix(name, "SecurityGroupId."):
			if g := srv.groups[values[0]]; g != nil {
				groups = append(groups, g)
			} else {
				fatalf(400, "InvalidGroup.NotFound", "unknown group id %q", values[0])
			}
		case strings.HasPrefix(name, "SecurityGroup."):
			var found *securityGroup
			for _, g := range srv.groups {
				if g.name == values[0] {
					found = g
				}
			}
			if found == nil {
				fatalf(400, "InvalidGroup.NotFound", "unknown group name %q", values[0])
			}
			groups = append(groups, found)
		}
	}
	return groups
}

func (srv *Server) group(group ec2.SecurityGroup) *securityGroup {
	if group.Id != "" {
		return srv.groups[group.Id]
	}
	for _, g := range srv.groups {
		if g.name == group.Name {
			return g
		}
	}
	return nil
}

func (srv *Server) createSecurityGroup(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	name := req.Form.Get("GroupName")
	if name == "" {
		fatalf(400, "InvalidParameterValue", "empty security group name")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.group(ec2.SecurityGroup{Name: name}) != nil {
		fatalf(400, "InvalidGroup.Duplicate", "group %q already exists", name)
	}
	g := &securityGroup{
		name:        name,
		description: req.Form.Get("GroupDescription"),
		id:          fmt.Sprintf("sg-%d", srv.groupId.next()),
		perms:       make(map[permKey]bool),
	}
	vpcId := req.Form.Get("VpcId")
	if vpcId != "" {
		g.vpcId = vpcId
	}
	srv.groups[g.id] = g
	// we define a local type for this because ec2.CreateSecurityGroupResp
	// contains SecurityGroup, but the response to this request
	// should not contain the security group name.
	type CreateSecurityGroupResponse struct {
		XMLName   xml.Name
		RequestId string `xml:"requestId"`
		Return    bool   `xml:"return"`
		GroupId   string `xml:"groupId"`
	}
	r := &CreateSecurityGroupResponse{
		XMLName:   xml.Name{defaultXMLName, "CreateSecurityGroupResponse"},
		RequestId: reqId,
		Return:    true,
		GroupId:   g.id,
	}
	return r
}

func (srv *Server) describeSecurityGroups(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	// BUG similar bug to describeInstances, but for GroupName and GroupId
	srv.mu.Lock()
	defer srv.mu.Unlock()

	var groups []*securityGroup
	for name, vals := range req.Form {
		var g ec2.SecurityGroup
		switch {
		case strings.HasPrefix(name, "GroupName."):
			g.Name = vals[0]
		case strings.HasPrefix(name, "GroupId."):
			g.Id = vals[0]
		default:
			continue
		}
		sg := srv.group(g)
		if sg == nil {
			fatalf(400, "InvalidGroup.NotFound", "no such group %v", g)
		}
		groups = append(groups, sg)
	}
	if len(groups) == 0 {
		for _, g := range srv.groups {
			groups = append(groups, g)
		}
	}

	f := newFilter(req.Form)
	var resp struct {
		XMLName   xml.Name
		RequestId string              `xml:"requestId"`
		Groups    []securityGroupInfo `xml:"securityGroupInfo>item"`
	}
	resp.XMLName = xml.Name{defaultXMLName, "DescribeSecurityGroupsResponse"}
	resp.RequestId = reqId
	for _, group := range groups {
		ok, err := f.ok(group)
		if ok {
			resp.Groups = append(resp.Groups, securityGroupInfo{
				OwnerId:       ownerId,
				SecurityGroup: group.ec2SecurityGroup(),
				Description:   group.description,
				IPPerms:       group.ec2Perms(),
			})
		} else if err != nil {
			fatalf(400, "InvalidParameterValue", "describe security groups: %v", err)
		}
	}
	return &resp
}

func (srv *Server) authorizeSecurityGroupIngress(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	g := srv.group(ec2.SecurityGroup{
		Name: req.Form.Get("GroupName"),
		Id:   req.Form.Get("GroupId"),
	})
	if g == nil {
		fatalf(400, "InvalidGroup.NotFound", "group not found")
	}
	perms := srv.parsePerms(req)

	for _, p := range perms {
		if g.perms[p] {
			fatalf(400, "InvalidPermission.Duplicate", "Permission has already been authorized on the specified group")
		}
	}
	for _, p := range perms {
		g.perms[p] = true
	}
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "AuthorizeSecurityGroupIngressResponse"},
		RequestId: reqId,
		Return:    true,
	}
}

func (srv *Server) revokeSecurityGroupIngress(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	g := srv.group(ec2.SecurityGroup{
		Name: req.Form.Get("GroupName"),
		Id:   req.Form.Get("GroupId"),
	})
	if g == nil {
		fatalf(400, "InvalidGroup.NotFound", "group not found")
	}
	perms := srv.parsePerms(req)

	// Note EC2 does not give an error if asked to revoke an authorization
	// that does not exist.
	for _, p := range perms {
		delete(g.perms, p)
	}
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "RevokeSecurityGroupIngressResponse"},
		RequestId: reqId,
		Return:    true,
	}
}

// parsePerms returns a slice of permKey values extracted
// from the permission fields in req.
func (srv *Server) parsePerms(req *http.Request) []permKey {
	// perms maps an index found in the form to its associated
	// IPPerm. For instance, the form value with key
	// "IpPermissions.3.FromPort" will be stored in perms[3].FromPort
	perms := make(map[int]ec2.IPPerm)

	type subgroupKey struct {
		id1, id2 int
	}
	// Each IPPerm can have many source security groups.  The form key
	// for a source security group contains two indices: the index
	// of the IPPerm and the sub-index of the security group. The
	// sourceGroups map maps from a subgroupKey containing these
	// two indices to the associated security group. For instance,
	// the form value with key "IPPermissions.3.Groups.2.GroupName"
	// will be stored in sourceGroups[subgroupKey{3, 2}].Name.
	sourceGroups := make(map[subgroupKey]ec2.UserSecurityGroup)

	// For each value in the form we store its associated information in the
	// above maps. The maps are necessary because the form keys may
	// arrive in any order, and the indices are not
	// necessarily sequential or even small.
	for name, vals := range req.Form {
		val := vals[0]
		var id1 int
		var rest string
		if x, _ := fmt.Sscanf(name, "IpPermissions.%d.%s", &id1, &rest); x != 2 {
			continue
		}
		ec2p := perms[id1]
		switch {
		case rest == "FromPort":
			ec2p.FromPort = atoi(val)
		case rest == "ToPort":
			ec2p.ToPort = atoi(val)
		case rest == "IpProtocol":
			switch val {
			case "tcp", "udp", "icmp":
				ec2p.Protocol = val
			default:
				// check it's a well formed number
				atoi(val)
				ec2p.Protocol = val
			}
		case strings.HasPrefix(rest, "Groups."):
			k := subgroupKey{id1: id1}
			if x, _ := fmt.Sscanf(rest[len("Groups."):], "%d.%s", &k.id2, &rest); x != 2 {
				continue
			}
			g := sourceGroups[k]
			switch rest {
			case "UserId":
				// BUG if the user id is blank, this does not conform to the
				// way that EC2 handles it - a specified but blank owner id
				// can cause RevokeSecurityGroupIngress to fail with
				// "group not found" even if the security group id has been
				// correctly specified.
				// By failing here, we ensure that we fail early in this case.
				if !ownerIdPat.MatchString(val) {
					fatalf(400, "InvalidUserID.Malformed", "Invalid user ID: %q", val)
				}
				g.OwnerId = val
			case "GroupName":
				g.Name = val
			case "GroupId":
				if !secGroupPat.MatchString(val) {
					fatalf(400, "InvalidGroupId.Malformed", "Invalid group ID: %q", val)
				}
				g.Id = val
			default:
				fatalf(400, "UnknownParameter", "unknown parameter %q", name)
			}
			sourceGroups[k] = g
		case strings.HasPrefix(rest, "IpRanges."):
			var id2 int
			if x, _ := fmt.Sscanf(rest[len("IpRanges."):], "%d.%s", &id2, &rest); x != 2 {
				continue
			}
			switch rest {
			case "CidrIp":
				if !cidrIpPat.MatchString(val) {
					fatalf(400, "InvalidParameterValue", "Invalid IP range: %q", val)
				}
				ec2p.SourceIPs = append(ec2p.SourceIPs, val)
			default:
				fatalf(400, "UnknownParameter", "unknown parameter %q", name)
			}
		default:
			fatalf(400, "UnknownParameter", "unknown parameter %q", name)
		}
		perms[id1] = ec2p
	}
	// Associate each set of source groups with its IPPerm.
	for k, g := range sourceGroups {
		p := perms[k.id1]
		p.SourceGroups = append(p.SourceGroups, g)
		perms[k.id1] = p
	}

	// Now that we have built up the IPPerms we need, we check for
	// parameter errors and build up a permKey for each permission,
	// looking up security groups from srv as we do so.
	var result []permKey
	for _, p := range perms {
		if p.FromPort > p.ToPort {
			fatalf(400, "InvalidParameterValue", "invalid port range")
		}
		k := permKey{
			protocol: p.Protocol,
			fromPort: p.FromPort,
			toPort:   p.ToPort,
		}
		for _, g := range p.SourceGroups {
			if g.OwnerId != "" && g.OwnerId != ownerId {
				fatalf(400, "InvalidGroup.NotFound", "group %q not found", g.Name)
			}
			var ec2g ec2.SecurityGroup
			switch {
			case g.Id != "":
				ec2g.Id = g.Id
			case g.Name != "":
				ec2g.Name = g.Name
			}
			k.group = srv.group(ec2g)
			if k.group == nil {
				fatalf(400, "InvalidGroup.NotFound", "group %v not found", g)
			}
			result = append(result, k)
		}
		k.group = nil
		for _, ip := range p.SourceIPs {
			k.ipAddr = ip
			result = append(result, k)
		}
	}
	return result
}

func (srv *Server) deleteSecurityGroup(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	g := srv.group(ec2.SecurityGroup{
		Name: req.Form.Get("GroupName"),
		Id:   req.Form.Get("GroupId"),
	})
	if g == nil {
		fatalf(400, "InvalidGroup.NotFound", "group not found")
	}
	for _, r := range srv.reservations {
		for _, h := range r.groups {
			if h == g && r.hasRunningMachine() {
				fatalf(500, "DependencyViolation", "group is currently in use by a running instance")
			}
		}
	}
	for _, sg := range srv.groups {
		// If a group refers to itself, it's ok to delete it.
		if sg == g {
			continue
		}
		for k := range sg.perms {
			if k.group == g {
				fatalf(500, "DependencyViolation", "group is currently in use by group %q", sg.id)
			}
		}
	}

	delete(srv.groups, g.id)
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "DeleteSecurityGroupResponse"},
		RequestId: reqId,
		Return:    true,
	}
}
