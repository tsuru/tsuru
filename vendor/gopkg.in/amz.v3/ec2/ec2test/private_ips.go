//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//
// This file contains code handling AWS API around private IP
// addresses for Elastic Network Interfaces.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"

	"gopkg.in/amz.v3/ec2"
)

func (srv *Server) addPrivateIPs(nic *iface, numToAdd int, addrs []string) error {
	for _, addr := range addrs {
		newIP := ec2.PrivateIP{Address: addr, IsPrimary: false}
		nic.PrivateIPs = append(nic.PrivateIPs, newIP)
	}
	if numToAdd == 0 {
		// Nothing more to do.
		return nil
	}
	firstIP := ""
	if len(nic.PrivateIPs) > 0 {
		firstIP = nic.PrivateIPs[len(nic.PrivateIPs)-1].Address
	} else {
		// Find the primary IP, if available, otherwise use
		// the subnet CIDR to generate a valid IP to use.
		firstIP = nic.PrivateIPAddress
		if firstIP == "" {
			sub := srv.subnets[nic.SubnetId]
			if sub == nil {
				return fmt.Errorf("NIC %q uses invalid subnet id: %v", nic.Id, nic.SubnetId)
			}
			netIP, _, err := net.ParseCIDR(sub.CIDRBlock)
			if err != nil {
				return fmt.Errorf("subnet %q has bad CIDR: %v", sub.Id, err)
			}
			firstIP = netIP.String()
		}
	}
	ip := net.ParseIP(firstIP)
	for i := 0; i < numToAdd; i++ {
		ip[len(ip)-1] += 1
		newIP := ec2.PrivateIP{Address: ip.String(), IsPrimary: false}
		nic.PrivateIPs = append(nic.PrivateIPs, newIP)
	}
	return nil
}

func (srv *Server) removePrivateIP(nic *iface, addr string) error {
	for i, privateIP := range nic.PrivateIPs {
		if privateIP.Address == addr {
			// Remove it, preserving order.
			nic.PrivateIPs = append(nic.PrivateIPs[:i], nic.PrivateIPs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("NIC %q does not have IP %q to remove", nic.Id, addr)
}

func (srv *Server) assignPrivateIP(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	nic := srv.iface(req.Form.Get("NetworkInterfaceId"))
	extraIPs := parseInOrder(req.Form, "PrivateIpAddress.")
	count := req.Form.Get("SecondaryPrivateIpAddressCount")
	secondaryIPs := 0
	if count != "" {
		secondaryIPs = atoi(count)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()

	err := srv.addPrivateIPs(nic, secondaryIPs, extraIPs)
	if err != nil {
		fatalf(400, "InvalidParameterValue", err.Error())
	}
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "AssignPrivateIpAddresses"},
		RequestId: reqId,
		Return:    true,
	}
}

func (srv *Server) unassignPrivateIP(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	nic := srv.iface(req.Form.Get("NetworkInterfaceId"))
	ips := parseInOrder(req.Form, "PrivateIpAddress.")

	srv.mu.Lock()
	defer srv.mu.Unlock()

	for _, ip := range ips {
		if err := srv.removePrivateIP(nic, ip); err != nil {
			fatalf(400, "InvalidParameterValue", err.Error())
		}
	}
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "UnassignPrivateIpAddresses"},
		RequestId: reqId,
		Return:    true,
	}
}
