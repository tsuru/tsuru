//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011-2015 Canonical Ltd.
//
// This file contains the Server type itself and a few other
// unexported methods, not fitting anywhere else.

package ec2test

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	"gopkg.in/amz.v3/ec2"
)

// TODO possible other things:
// - some virtual time stamp interface, so a client
// can ask for all actions after a certain virtual time.

// Server implements an EC2 simulator for use in testing.
type Server struct {
	url             string
	listener        net.Listener
	mu              sync.Mutex
	reqs            []*Action
	createRootDisks bool

	attributes           map[string][]string             // attr name -> values
	instances            map[string]*Instance            // id -> instance
	reservations         map[string]*reservation         // id -> reservation
	groups               map[string]*securityGroup       // id -> group
	zones                map[string]availabilityZone     // name -> availabilityZone
	vpcs                 map[string]*vpc                 // id -> vpc
	internetGateways     map[string]*internetGateway     // id -> igw
	routeTables          map[string]*routeTable          // id -> table
	subnets              map[string]*subnet              // id -> subnet
	ifaces               map[string]*iface               // id -> iface
	networkAttachments   map[string]*interfaceAttachment // id -> attachment
	volumes              map[string]*volume              // id -> volume
	volumeAttachments    map[string]*volumeAttachment    // id -> volumeAttachment
	maxId                counter
	reqId                counter
	reservationId        counter
	groupId              counter
	vpcId                counter
	igwId                counter
	rtbId                counter
	rtbassocId           counter
	dhcpOptsId           counter
	subnetId             counter
	volumeId             counter
	ifaceId              counter
	attachId             counter
	initialInstanceState ec2.InstanceState
}

// NewServer returns a new server.
func NewServer() (*Server, error) {
	srv := &Server{}
	srv.Reset(false)

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("cannot listen on localhost: %v", err)
	}
	srv.listener = l

	srv.url = "http://" + l.Addr().String()

	// we use HandlerFunc rather than *Server directly so that we
	// can avoid exporting HandlerFunc from *Server.
	go http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		srv.serveHTTP(w, req)
	}))
	return srv, nil
}

// Reset is a convenient helper to remove all test entities (e.g.
// VPCs, subnets, instances) from the test server and reset all id
// counters. The, if withoutZonesOrGroups is false, a default AZ and
// security group will be created.
func (srv *Server) Reset(withoutZonesOrGroups bool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	srv.maxId.reset()
	srv.reqId.reset()
	srv.reservationId.reset()
	srv.groupId.reset()
	srv.vpcId.reset()
	srv.igwId.reset()
	srv.rtbId.reset()
	srv.rtbassocId.reset()
	srv.dhcpOptsId.reset()
	srv.subnetId.reset()
	srv.volumeId.reset()
	srv.ifaceId.reset()
	srv.attachId.reset()

	srv.attributes = make(map[string][]string)
	srv.instances = make(map[string]*Instance)
	srv.groups = make(map[string]*securityGroup)
	srv.vpcs = make(map[string]*vpc)
	srv.zones = make(map[string]availabilityZone)
	srv.internetGateways = make(map[string]*internetGateway)
	srv.routeTables = make(map[string]*routeTable)
	srv.subnets = make(map[string]*subnet)
	srv.ifaces = make(map[string]*iface)
	srv.networkAttachments = make(map[string]*interfaceAttachment)
	srv.volumes = make(map[string]*volume)
	srv.volumeAttachments = make(map[string]*volumeAttachment)
	srv.reservations = make(map[string]*reservation)

	srv.reqs = []*Action{}
	if !withoutZonesOrGroups {
		srv.addDefaultZonesAndGroups()
	}
}

// Quit closes down the server.
func (srv *Server) Quit() {
	srv.listener.Close()
}

// URL returns the URL of the server.
func (srv *Server) URL() string {
	return srv.url
}
