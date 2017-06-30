//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2015 Canonical Ltd.
//

package ec2

import (
	"strconv"
)

// The CreateVolume type encapsulates options for the respective request in EC2.
//
// See http://goo.gl/uXCf9F for more details.
type CreateVolume struct {
	AvailZone  string
	SnapshotId string
	VolumeType string
	VolumeSize int // Size is given in GB
	Encrypted  bool

	// The number of I/O operations per second (IOPS) that the volume supports.
	IOPS int64
}

// Volume describes an Amazon Volume.
//
// See http://goo.gl/3jssTQ for more details.
type Volume struct {
	Id          string             `xml:"volumeId"`
	Size        int                `xml:"size"`
	SnapshotId  string             `xml:"snapshotId"`
	Status      string             `xml:"status"`
	IOPS        int64              `xml:"iops"`
	AvailZone   string             `xml:"availabilityZone"`
	CreateTime  string             `xml:"createTime"`
	VolumeType  string             `xml:"volumeType"`
	Encrypted   bool               `xml:"encrypted"`
	Tags        []Tag              `xml:"tagSet>item"`
	Attachments []VolumeAttachment `xml:"attachmentSet>item"`
}

// VolumeAttachment describes an Amazon Volume Attachment.
//
// See http://goo.gl/DLkRxx for more details.
type VolumeAttachment struct {
	VolumeId            string `xml:"volumeId"`
	Device              string `xml:"device"`
	InstanceId          string `xml:"instanceId"`
	Status              string `xml:"status"`
	DeleteOnTermination bool   `xml:"deleteOnTermination"`
}

// CreateVolumeResp is the response to a CreateVolume request.
//
// See http://goo.gl/uXCf9F for more details.
type CreateVolumeResp struct {
	RequestId string `xml:"requestId"`
	Volume
}

// CreateVolume creates a subnet in an existing VPC.
//
// See http://goo.gl/uXCf9F for more details.
func (ec2 *EC2) CreateVolume(volume CreateVolume) (resp *CreateVolumeResp, err error) {
	params := makeParams("CreateVolume")
	prepareVolume(params, volume)
	resp = &CreateVolumeResp{}
	err = ec2.query(params, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func prepareVolume(params map[string]string, volume CreateVolume) {
	params["AvailabilityZone"] = volume.AvailZone
	if volume.SnapshotId != "" {
		params["SnapshotId"] = volume.SnapshotId
	}
	if volume.VolumeType != "" {
		params["VolumeType"] = volume.VolumeType
	}
	if volume.VolumeSize > 0 {
		params["Size"] = strconv.FormatInt(int64(volume.VolumeSize), 10)
	}
	if volume.IOPS > 0 {
		params["Iops"] = strconv.FormatInt(volume.IOPS, 10)
	}
	if volume.Encrypted {
		params["Encrypted"] = "true"
	}
}

// DeleteVolume deletes the specified volume.
// The volume must be in the available state (not attached to an instance).

// See http://goo.gl/AM46X0 for more details.
func (ec2 *EC2) DeleteVolume(id string) (resp *SimpleResp, err error) {
	params := makeParams("DeleteVolume")
	params["VolumeId"] = id
	resp = &SimpleResp{}
	err = ec2.query(params, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// VolumesResp is the response to a Volumes request.
//
// See http://goo.gl/NTKQVI for more details.
type VolumesResp struct {
	RequestId string   `xml:"requestId"`
	Volumes   []Volume `xml:"volumeSet>item"`
}

// Volumes returns one or more volumes. Both parameters are optional,
// and if specified will limit the returned volumes to the matching
// ids or filtering rules.
//
// See http://goo.gl/cZCJM4 for more details.
func (ec2 *EC2) Volumes(ids []string, filter *Filter) (resp *VolumesResp, err error) {
	params := makeParams("DescribeVolumes")
	for i, id := range ids {
		params["VolumeId."+strconv.Itoa(i+1)] = id
	}
	filter.addParams(params)

	resp = &VolumesResp{}
	err = ec2.query(params, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// VolumeAttachmentResp is the response to a VolumeAttachment request (attach/delete).
//
// See http://goo.gl/oESpiF for more details.
type VolumeAttachmentResp struct {
	RequestId  string `xml:"requestId"`
	VolumeId   string `xml:"volumeId"`
	Device     string `xml:"device"`
	InstanceId string `xml:"instanceId"`
	Status     string `xml:"status"`
	AttachTime string `xml:"attachTime"`
}

// AttachVolume attaches an Amazon EBS volume to a running or stopped instance
// and exposes it to the instance with the specified device name.

// See http://goo.gl/oESpiF for more details.
func (ec2 *EC2) AttachVolume(volumeId, instanceId, device string) (resp *VolumeAttachmentResp, err error) {
	params := makeParams("AttachVolume")
	params["VolumeId"] = volumeId
	params["InstanceId"] = instanceId
	params["Device"] = device
	resp = &VolumeAttachmentResp{}
	err = ec2.query(params, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// DetachVolume detaches an Amazon EBS volume from an instance
// and exposes it to the instance with the specified device name.

// See http://goo.gl/22mwZP for more details.
func (ec2 *EC2) DetachVolume(volumeId, instanceId, device string, force bool) (resp *VolumeAttachmentResp, err error) {
	params := makeParams("DetachVolume")
	params["VolumeId"] = volumeId
	params["InstanceId"] = instanceId
	params["Device"] = device
	if force {
		params["Force"] = "true"
	}
	resp = &VolumeAttachmentResp{}
	err = ec2.query(params, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
