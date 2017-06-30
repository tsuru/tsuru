//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011-2015 Canonical Ltd.
//
// This file contains code handling AWS API around Network Interface
// Attachments.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"gopkg.in/amz.v3/ec2"
)

type interfaceAttachment struct {
	ec2.NetworkInterfaceAttachment
}

func (srv *Server) attachIFace(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	i := srv.iface(req.Form.Get("NetworkInterfaceId"))
	inst := srv.instance(req.Form.Get("InstanceId"))
	devIndex := atoi(req.Form.Get("DeviceIndex"))

	srv.mu.Lock()
	defer srv.mu.Unlock()
	a := &interfaceAttachment{ec2.NetworkInterfaceAttachment{
		Id:                  fmt.Sprintf("eni-attach-%d", srv.attachId.next()),
		InstanceId:          inst.id(),
		InstanceOwnerId:     ownerId,
		DeviceIndex:         devIndex,
		Status:              "attached",
		AttachTime:          time.Now().Format(time.RFC3339),
		DeleteOnTermination: false, // false for manually created NICs
	}}
	srv.networkAttachments[a.Id] = a
	i.Attachment = a.NetworkInterfaceAttachment
	i.Status = "in-use"
	srv.ifaces[i.Id] = i
	var resp struct {
		XMLName xml.Name
		ec2.AttachNetworkInterfaceResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "AttachNetworkInterfaceResponse"}
	resp.RequestId = reqId
	resp.AttachmentId = a.Id
	return resp
}

func (srv *Server) detachIFace(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	att := srv.interfaceAttachment(req.Form.Get("AttachmentId"))

	srv.mu.Lock()
	defer srv.mu.Unlock()

	for _, i := range srv.ifaces {
		if i.Attachment.Id == att.Id {
			i.Attachment = ec2.NetworkInterfaceAttachment{}
			srv.ifaces[i.Id] = i
			break
		}
	}
	delete(srv.networkAttachments, att.Id)
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "DetachNetworkInterfaceResponse"},
		RequestId: reqId,
		Return:    true,
	}
}

func (srv *Server) interfaceAttachment(id string) *interfaceAttachment {
	if id == "" {
		fatalf(400, "MissingParameter", "missing attachmentId")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	att, found := srv.networkAttachments[id]
	if !found {
		fatalf(
			400,
			"InvalidNetworkInterfaceAttachmentId.NotFound",
			"interface attachment %s not found", id,
		)
	}
	return att
}
