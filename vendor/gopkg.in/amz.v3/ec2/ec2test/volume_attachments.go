//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//
// This file contains code handling AWS API around Volume Attachments.

package ec2test

import (
	"encoding/xml"
	"net/http"
	"time"

	"gopkg.in/amz.v3/ec2"
)

type volumeAttachment struct {
	ec2.VolumeAttachment
}

func (srv *Server) attachVolume(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	if req.Form.Get("VolumeId") == "" {
		fatalf(400, "MissingParameter", "missing volume id")
	}
	if req.Form.Get("InstanceId") == "" {
		fatalf(400, "MissingParameter", "missing instance id")
	}
	if req.Form.Get("Device") == "" {
		fatalf(400, "MissingParameter", "missing device")
	}
	ec2VolAttachment := srv.parseVolumeAttachment(req)

	if _, ok := srv.volumeAttachments[ec2VolAttachment.VolumeId]; ok {
		fatalf(400, "VolumeInUse", "Volume %s is already attached", ec2VolAttachment.VolumeId)
	}
	v := srv.volume(ec2VolAttachment.VolumeId)

	srv.mu.Lock()
	defer srv.mu.Unlock()
	va := &volumeAttachment{ec2VolAttachment}
	va.Status = "attached"
	v.Status = "in-use"
	srv.volumeAttachments[va.VolumeId] = va
	var resp struct {
		XMLName xml.Name
		ec2.VolumeAttachmentResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "AttachVolumeResponse"}
	resp.RequestId = reqId
	resp.VolumeId = va.VolumeId
	resp.InstanceId = va.InstanceId
	resp.Device = va.Device
	resp.Status = va.Status
	resp.AttachTime = time.Now().Format(time.RFC3339)
	return resp
}

func (srv *Server) parseVolumeAttachment(req *http.Request) ec2.VolumeAttachment {
	attachment := ec2.VolumeAttachment{}
	var vol *volume
	var inst *Instance
	for attr, vals := range req.Form {
		switch attr {
		case "AWSAccessKeyId", "Action", "Signature", "SignatureMethod", "SignatureVersion",
			"Version", "Timestamp":
			continue
		case "VolumeId":
			v := vals[0]
			// Check volume id validity.
			vol = srv.volume(v)
			if vol.Status != "available" {
				fatalf(400, " IncorrectState", "cannot attach volume that is not available", v)
			}
			attachment.VolumeId = v
		case "InstanceId":
			v := vals[0]
			// Check instance id validity.
			inst = srv.instance(v)
			if inst.state != Running {
				fatalf(400, "IncorrectInstanceState", "cannot attach volume to instance %s as it is not running", v)
			}
			attachment.InstanceId = v
		case "Device":
			attachment.Device = vals[0]
		default:
			fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
		}
	}
	if vol.AvailZone != inst.availZone {
		fatalf(
			400,
			"InvalidVolume.ZoneMismatch",
			"volume availability zone %q must match instance zone %q", vol.AvailZone, inst.availZone,
		)
	}
	return attachment
}

func (srv *Server) volumeAttachment(id string) *volumeAttachment {
	if id == "" {
		fatalf(400, "MissingParameter", "missing volumeId")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	v, found := srv.volumeAttachments[id]
	if !found {
		fatalf(400, "InvalidAttachment.NotFound", "Volume attachment for volume %s not found", id)
	}
	return v
}

func (srv *Server) detachVolume(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	vId := req.Form.Get("VolumeId")
	// Get attachment first so if not found, the expected error is returned.
	va := srv.volumeAttachment(vId)
	// Validate volume exists.
	v := srv.volume(vId)

	srv.mu.Lock()
	defer srv.mu.Unlock()
	delete(srv.volumeAttachments, vId)
	v.Status = "available"
	var resp struct {
		XMLName xml.Name
		ec2.VolumeAttachmentResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "DetachVolumeResponse"}
	resp.RequestId = reqId
	resp.VolumeId = va.VolumeId
	resp.InstanceId = va.InstanceId
	resp.Device = va.Device
	resp.Status = "detaching"
	return resp
}
