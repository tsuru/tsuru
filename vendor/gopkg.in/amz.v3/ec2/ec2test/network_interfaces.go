//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011-2015 Canonical Ltd.
//
// This file contains code handling AWS API around Elastic Network
// Interfaces.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gopkg.in/amz.v3/ec2"
)

type iface struct {
	ec2.NetworkInterface
}

func (i *iface) matchAttr(attr, value string) (ok bool, err error) {
	notImplemented := []string{
		"addresses.association.", "association.", "tag", "requester-",
	}
	switch attr {
	case "availability-zone":
		return i.AvailZone == value, nil
	case "network-interface-id":
		return i.Id == value, nil
	case "status":
		return i.Status == value, nil
	case "subnet-id":
		return i.SubnetId == value, nil
	case "vpc-id":
		return i.VPCId == value, nil
	case "attachment.attachment-id":
		return i.Attachment.Id == value, nil
	case "attachment.instance-id":
		return i.Attachment.InstanceId == value, nil
	case "attachment.instance-owner-id":
		return i.Attachment.InstanceOwnerId == value, nil
	case "attachment.device-index":
		devIndex, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return i.Attachment.DeviceIndex == devIndex, nil
	case "attachment.status":
		return i.Attachment.Status == value, nil
	case "attachment.attach-time":
		return i.Attachment.AttachTime == value, nil
	case "attachment.delete-on-termination":
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		// EC2 only filters attached NICs here, as the flag defaults
		// to false for manually created NICs and to true for
		// automatically created ones (during RunInstances)
		if i.Attachment.Id == "" {
			return false, nil
		}
		return i.Attachment.DeleteOnTermination == flag, nil
	case "owner-id":
		return i.OwnerId == value, nil
	case "source-dest-check":
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		return i.SourceDestCheck == flag, nil
	case "description":
		return i.Description == value, nil
	case "private-dns-name":
		return i.PrivateDNSName == value, nil
	case "mac-address":
		return i.MACAddress == value, nil
	case "private-ip-address", "addresses.private-ip-address":
		if i.PrivateIPAddress == value {
			return true, nil
		}
		// Look inside the secondary IPs list.
		for _, ip := range i.PrivateIPs {
			if ip.Address == value {
				return true, nil
			}
		}
		return false, nil
	case "addresses.primary":
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		for _, ip := range i.PrivateIPs {
			if ip.IsPrimary == flag {
				return true, nil
			}
		}
		return false, nil
	case "group-id":
		for _, group := range i.Groups {
			if group.Id == value {
				return true, nil
			}
		}
		return false, nil
	case "group-name":
		for _, group := range i.Groups {
			if group.Name == value {
				return true, nil
			}
		}
		return false, nil
	default:
		for _, item := range notImplemented {
			if strings.HasPrefix(attr, item) {
				return false, fmt.Errorf("%q filter not implemented", attr)
			}
		}
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

// parseRunNetworkInterfaces parses any RunNetworkInterface parameters
// passed to RunInstances, and returns them, along with a
// limitToOneInstance flag. The flag is set only when an existing
// network interface id is specified, which according to the API
// limits the number of instances to 1.
func (srv *Server) parseRunNetworkInterfaces(req *http.Request) ([]ec2.RunNetworkInterface, bool) {
	ifaces := []ec2.RunNetworkInterface{}
	limitToOneInstance := false
	for attr, vals := range req.Form {
		if !strings.HasPrefix(attr, "NetworkInterface.") {
			// Only process network interface params.
			continue
		}
		fields := strings.Split(attr, ".")
		if len(fields) < 3 || len(vals) != 1 {
			fatalf(400, "InvalidParameterValue", "bad param %q: %v", attr, vals)
		}
		index := atoi(fields[1])
		// Field name format: NetworkInterface.<index>.<fieldName>....
		for len(ifaces)-1 < index {
			ifaces = append(ifaces, ec2.RunNetworkInterface{})
		}
		iface := ifaces[index]
		fieldName := fields[2]
		switch fieldName {
		case "NetworkInterfaceId":
			// We're using an existing NIC, hence only a single
			// instance can be launched.
			id := vals[0]
			if _, ok := srv.ifaces[id]; !ok {
				fatalf(400, "InvalidNetworkInterfaceID.NotFound", "no such nic id %q", id)
			}
			iface.Id = id
			limitToOneInstance = true
		case "DeviceIndex":
			// This applies both when creating a new NIC and when using an existing one.
			iface.DeviceIndex = atoi(vals[0])
		case "SubnetId":
			// We're creating a new NIC (from here on).
			id := vals[0]
			if _, ok := srv.subnets[id]; !ok {
				fatalf(400, "InvalidSubnetID.NotFound", "no such subnet id %q", id)
			}
			iface.SubnetId = id
		case "Description":
			iface.Description = vals[0]
		case "DeleteOnTermination":
			val, err := strconv.ParseBool(vals[0])
			if err != nil {
				fatalf(400, "InvalidParameterValue", "bad flag %s: %s", fieldName, vals[0])
			}
			iface.DeleteOnTermination = val
		case "PrivateIpAddress":
			privateIP := ec2.PrivateIP{
				Address:   vals[0],
				DNSName:   srv.dnsNameFromPrivateIP(vals[0]),
				IsPrimary: true,
			}
			iface.PrivateIPs = append(iface.PrivateIPs, privateIP)
			// When a single private IP address is explicitly specified,
			// only one instance can be launched according to the API docs.
			limitToOneInstance = true
		case "SecondaryPrivateIpAddressCount":
			iface.SecondaryPrivateIPCount = atoi(vals[0])
		case "PrivateIpAddresses":
			// ...PrivateIpAddress.<ipIndex>.<subFieldName>: vals[0]
			if len(fields) < 4 {
				fatalf(400, "InvalidParameterValue", "bad param %q: %v", attr, vals)
			}
			ipIndex := atoi(fields[3])
			for len(iface.PrivateIPs)-1 < ipIndex {
				iface.PrivateIPs = append(iface.PrivateIPs, ec2.PrivateIP{})
			}
			privateIP := iface.PrivateIPs[ipIndex]
			subFieldName := fields[4]
			switch subFieldName {
			case "PrivateIpAddress":
				privateIP.Address = vals[0]
				privateIP.DNSName = srv.dnsNameFromPrivateIP(vals[0])
			case "Primary":
				val, err := strconv.ParseBool(vals[0])
				if err != nil {
					fatalf(400, "InvalidParameterValue", "bad flag %s: %s", subFieldName, vals[0])
				}
				privateIP.IsPrimary = val
			default:
				fatalf(400, "InvalidParameterValue", "unknown field %s, subfield %s: %s", fieldName, subFieldName, vals[0])
			}
			iface.PrivateIPs[ipIndex] = privateIP
		case "SecurityGroupId":
			// ...SecurityGroupId.<#>:  <sgId>
			for _, sgId := range vals {
				if _, ok := srv.groups[sgId]; !ok {
					fatalf(400, "InvalidParameterValue", "no such security group id %q", sgId)
				}
				iface.SecurityGroupIds = append(iface.SecurityGroupIds, sgId)
			}
		default:
			fatalf(400, "InvalidParameterValue", "unknown field %s: %s", fieldName, vals[0])
		}
		ifaces[index] = iface
	}
	return ifaces, limitToOneInstance
}

// addDefaultNIC requests the creation of a default network interface
// for each instance to launch in RunInstances, using the given
// instance subnet, and it's called when no explicit NICs are given.
// It returns the populated RunNetworkInterface slice to add to
// RunInstances params.
func (srv *Server) addDefaultNIC(instSubnet *subnet) []ec2.RunNetworkInterface {
	if instSubnet == nil {
		// No subnet specified, nothing to do.
		return nil
	}

	ip, ipnet, err := net.ParseCIDR(instSubnet.CIDRBlock)
	if err != nil {
		panic(fmt.Sprintf("subnet %q has invalid CIDR: %v", instSubnet.Id, err.Error()))
	}
	// Just pick a valid subnet IP, it doesn't have to be unique
	// across instances, as this is a testing server.
	ip[len(ip)-1] = 5
	if !ipnet.Contains(ip) {
		panic(fmt.Sprintf("%q does not contain IP %q", instSubnet.Id, ip))
	}
	return []ec2.RunNetworkInterface{{
		Id:                  fmt.Sprintf("eni-%d", srv.ifaceId.next()),
		DeviceIndex:         0,
		Description:         "created by ec2test server",
		DeleteOnTermination: true,
		PrivateIPs: []ec2.PrivateIP{{
			Address:   ip.String(),
			DNSName:   srv.dnsNameFromPrivateIP(ip.String()),
			IsPrimary: true,
		}},
	}}
}

// createNICsOnRun creates and returns any network interfaces
// specified in ifacesToCreate in the server for the given instance id
// and subnet.
func (srv *Server) createNICsOnRun(instId string, instSubnet *subnet, ifacesToCreate []ec2.RunNetworkInterface) []ec2.NetworkInterface {
	if instSubnet == nil {
		// No subnet specified, nothing to do.
		return nil
	}

	var createdNICs []ec2.NetworkInterface
	for _, ifaceToCreate := range ifacesToCreate {
		nicId := ifaceToCreate.Id
		macAddress := fmt.Sprintf("20:%02x:60:cb:27:37", srv.ifaceId.get())
		if nicId == "" {
			// Simulate a NIC got created.
			nicId = fmt.Sprintf("eni-%d", srv.ifaceId.next())
			macAddress = fmt.Sprintf("20:%02x:60:cb:27:37", srv.ifaceId.get())
		}
		groups := make([]ec2.SecurityGroup, len(ifaceToCreate.SecurityGroupIds))
		for i, sgId := range ifaceToCreate.SecurityGroupIds {
			groups[i] = srv.group(ec2.SecurityGroup{Id: sgId}).ec2SecurityGroup()
		}
		// Find the primary private IP address for the NIC
		// inside the PrivateIPs slice.
		primaryIP := ""
		for i, ip := range ifaceToCreate.PrivateIPs {
			if ip.IsPrimary {
				primaryIP = ip.Address
				dnsName := srv.dnsNameFromPrivateIP(primaryIP)
				ifaceToCreate.PrivateIPs[i].DNSName = dnsName
				break
			}
		}
		attach := ec2.NetworkInterfaceAttachment{
			Id:                  fmt.Sprintf("eni-attach-%d", srv.attachId.next()),
			InstanceId:          instId,
			InstanceOwnerId:     ownerId,
			DeviceIndex:         ifaceToCreate.DeviceIndex,
			Status:              "in-use",
			AttachTime:          time.Now().Format(time.RFC3339),
			DeleteOnTermination: true,
		}
		srv.networkAttachments[attach.Id] = &interfaceAttachment{attach}
		nic := ec2.NetworkInterface{
			Id:               nicId,
			SubnetId:         instSubnet.Id,
			VPCId:            instSubnet.VPCId,
			AvailZone:        instSubnet.AvailZone,
			Description:      ifaceToCreate.Description,
			OwnerId:          ownerId,
			Status:           "in-use",
			MACAddress:       macAddress,
			PrivateIPAddress: primaryIP,
			PrivateDNSName:   srv.dnsNameFromPrivateIP(primaryIP),
			SourceDestCheck:  true,
			Groups:           groups,
			PrivateIPs:       ifaceToCreate.PrivateIPs,
			Attachment:       attach,
		}
		srv.ifaces[nicId] = &iface{nic}
		createdNICs = append(createdNICs, nic)
	}
	return createdNICs
}

func (srv *Server) createIFace(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	s := srv.subnet(req.Form.Get("SubnetId"))
	ipMap := make(map[int]ec2.PrivateIP)
	primaryIP := req.Form.Get("PrivateIpAddress")
	if primaryIP != "" {
		ipMap[0] = ec2.PrivateIP{
			Address:   primaryIP,
			DNSName:   srv.dnsNameFromPrivateIP(primaryIP),
			IsPrimary: true,
		}
	}
	desc := req.Form.Get("Description")

	var groups []ec2.SecurityGroup
	for name, vals := range req.Form {
		if strings.HasPrefix(name, "SecurityGroupId.") {
			g := ec2.SecurityGroup{Id: vals[0]}
			sg := srv.group(g)
			if sg == nil {
				fatalf(400, "InvalidGroup.NotFound", "no such group %v", g)
			}
			groups = append(groups, sg.ec2SecurityGroup())
		}
		if strings.HasPrefix(name, "PrivateIpAddresses.") {
			var ip ec2.PrivateIP
			parts := strings.Split(name, ".")
			index := atoi(parts[1]) - 1
			if index < 0 {
				fatalf(400, "InvalidParameterValue", "invalid index %s", name)
			}
			if _, ok := ipMap[index]; ok {
				ip = ipMap[index]
			}
			switch parts[2] {
			case "PrivateIpAddress":
				ip.Address = vals[0]
				ip.DNSName = srv.dnsNameFromPrivateIP(ip.Address)
			case "Primary":
				val, err := strconv.ParseBool(vals[0])
				if err != nil {
					fatalf(400, "InvalidParameterValue", "bad flag %s: %s", name, vals[0])
				}
				ip.IsPrimary = val
			}
			ipMap[index] = ip
		}
	}
	privateIPs := make([]ec2.PrivateIP, len(ipMap))
	for index, ip := range ipMap {
		if ip.IsPrimary {
			primaryIP = ip.Address
		}
		privateIPs[index] = ip
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	i := &iface{ec2.NetworkInterface{
		Id:               fmt.Sprintf("eni-%d", srv.ifaceId.next()),
		SubnetId:         s.Id,
		VPCId:            s.VPCId,
		AvailZone:        s.AvailZone,
		Description:      desc,
		OwnerId:          ownerId,
		Status:           "available",
		MACAddress:       fmt.Sprintf("20:%02x:60:cb:27:37", srv.ifaceId.get()),
		PrivateIPAddress: primaryIP,
		PrivateDNSName:   srv.dnsNameFromPrivateIP(primaryIP),
		SourceDestCheck:  true,
		Groups:           groups,
		PrivateIPs:       privateIPs,
	}}
	srv.ifaces[i.Id] = i
	var resp struct {
		XMLName xml.Name
		ec2.CreateNetworkInterfaceResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "CreateNetworkInterfaceResponse"}
	resp.RequestId = reqId
	resp.NetworkInterface = i.NetworkInterface
	return resp
}

func (srv *Server) dnsNameFromPrivateIP(privateIP string) string {
	return fmt.Sprintf("ip-%s.ec2.internal", strings.Replace(privateIP, ".", "-", -1))
}

func (srv *Server) deleteIFace(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	i := srv.iface(req.Form.Get("NetworkInterfaceId"))

	srv.mu.Lock()
	defer srv.mu.Unlock()

	delete(srv.ifaces, i.Id)
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "DeleteNetworkInterfaceResponse"},
		RequestId: reqId,
		Return:    true,
	}
}

func (srv *Server) describeIFaces(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	idMap := parseIDs(req.Form, "NetworkInterfaceId.")
	for id, _ := range idMap {
		if _, known := srv.ifaces[id]; !known {
			fatalf(400, "InvalidNetworkInterfaceID.NotFound", "no such NIC %v", id)
		}
	}
	f := newFilter(req.Form)
	var resp struct {
		XMLName xml.Name
		ec2.NetworkInterfacesResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "DescribeNetworkInterfacesResponse"}
	resp.RequestId = reqId
	for _, i := range srv.ifaces {
		filterMatch, err := f.ok(i)
		if err != nil {
			fatalf(400, "InvalidParameterValue", "describe ifaces: %v", err)
		}
		if filterMatch && (len(idMap) == 0 || idMap[i.Id]) {
			// filter.ok() returns true when the filter is empty.
			resp.Interfaces = append(resp.Interfaces, i.NetworkInterface)
		}
	}
	return &resp
}

func (srv *Server) iface(id string) *iface {
	if id == "" {
		fatalf(400, "MissingParameter", "missing networkInterfaceId")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	i, found := srv.ifaces[id]
	if !found {
		fatalf(400, "InvalidNetworkInterfaceID.NotFound", "interface %s not found", id)
	}
	return i
}
