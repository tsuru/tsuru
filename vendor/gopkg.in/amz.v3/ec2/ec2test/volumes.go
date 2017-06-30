//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//
// This file contains code handling AWS API around Volumes.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gopkg.in/amz.v3/ec2"
)

// SetCreateRootDisks records whether or not the server should create
// root disks for each instance created. It defaults to false.
func (srv *Server) SetCreateRootDisks(create bool) {
	srv.mu.Lock()
	srv.createRootDisks = create
	srv.mu.Unlock()
}

func (srv *Server) newVolume(volumeType string, size int) *volume {
	// Create a volume and volume attachment too.
	volume := &volume{}
	volume.Id = fmt.Sprintf("vol-%d", srv.volumeId.next())
	volume.Status = "available"
	volume.CreateTime = time.Now().Format(time.RFC3339)
	volume.VolumeType = volumeType
	volume.Size = size
	srv.volumes[volume.Id] = volume
	return volume
}

type volume struct {
	ec2.Volume
}

func (v *volume) matchAttr(attr, value string) (ok bool, err error) {
	if strings.HasPrefix(attr, "attachment.") {
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	if strings.HasPrefix(attr, "tag:") {
		key := attr[len("tag:"):]
		return matchTag(v.Tags, key, value), nil
	}
	switch attr {
	case "volume-type":
		return v.VolumeType == value, nil
	case "status":
		return v.Status == value, nil
	case "volume-id":
		return v.Id == value, nil
	case "size":
		size, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return v.Size == size, nil
	case "availability-zone":
		return v.AvailZone == value, nil
	case "snapshot-id":
		return v.SnapshotId == value, nil
	case "encrypted":
		encrypted, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		return v.Encrypted == encrypted, nil
	case "tag", "tag-key", "tag-value", "create-time":
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (srv *Server) createVolume(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	if req.Form.Get("AvailabilityZone") == "" {
		fatalf(400, "MissingParameter", "missing availability zone")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	ec2Vol := srv.parseVolume(req)
	v := &volume{ec2Vol}
	srv.volumes[v.Id] = v
	var resp struct {
		XMLName xml.Name
		ec2.CreateVolumeResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "CreateVolumeResponse"}
	resp.RequestId = reqId
	resp.Volume = v.Volume
	return resp
}

func (srv *Server) parseVolume(req *http.Request) ec2.Volume {
	volume := srv.newVolume("magnetic", 1)
	for attr, vals := range req.Form {
		switch attr {
		case "AWSAccessKeyId", "Action", "Signature", "SignatureMethod", "SignatureVersion",
			"Version", "Timestamp":
			continue
		case "AvailabilityZone":
			v := vals[0]
			if v == "" {
				fatalf(400, "MissingParameter", "missing availability zone")
			}
			volume.AvailZone = v
		case "SnapshotId":
			volume.SnapshotId = vals[0]
		case "VolumeType":
			volume.VolumeType = vals[0]
		case "Size":
			volume.Size = atoi(vals[0])
		case "Iops":
			volume.IOPS = int64(atoi(vals[0]))
		case "Encrypted":
			val, err := strconv.ParseBool(vals[0])
			if err != nil {
				fatalf(400, "InvalidParameterValue", "bad flag %s: %s", attr, vals[0])
			}
			volume.Encrypted = val
		default:
			fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
		}
	}
	return volume.Volume
}

func (srv *Server) deleteVolume(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	v := srv.volume(req.Form.Get("VolumeId"))
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if _, ok := srv.volumeAttachments[v.Id]; ok {
		fatalf(400, "VolumeInUse", "Volume %s is attached", v.Id)
	}
	delete(srv.volumes, v.Id)
	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "DeleteVolumeResponse"},
		RequestId: reqId,
		Return:    true,
	}
}

func (srv *Server) volume(id string) *volume {
	if id == "" {
		fatalf(400, "MissingParameter", "missing volumeId")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	v, found := srv.volumes[id]
	if !found {
		fatalf(400, "InvalidVolume.NotFound", "Volume %s not found", id)
	}
	return v
}

func (srv *Server) describeVolumes(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	idMap := parseIDs(req.Form, "VolumeId.")
	f := newFilter(req.Form)
	var resp struct {
		XMLName xml.Name
		ec2.VolumesResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "DescribeVolumesResponse"}
	resp.RequestId = reqId
	for _, v := range srv.volumes {
		ok, err := f.ok(v)
		_, known := idMap[v.Id]
		if ok && (len(idMap) == 0 || known) {
			vol := v.Volume
			if va, ok := srv.volumeAttachments[v.Id]; ok {
				vol.Attachments = []ec2.VolumeAttachment{va.VolumeAttachment}
			}
			resp.Volumes = append(resp.Volumes, vol)
		} else if err != nil {
			fatalf(400, "InvalidParameterValue", "describe Volumes: %v", err)
		}
	}
	return &resp
}
