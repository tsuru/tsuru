//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011-2015 Canonical Ltd.
//
// This file contains code handing block devices with AWS API.

package ec2test

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gopkg.in/amz.v3/ec2"
)

// parseBlockDeviceMappings parses and returns any BlockDeviceMapping
// parameters passed to RunInstances.
func (srv *Server) parseBlockDeviceMappings(req *http.Request) []ec2.BlockDeviceMapping {
	mappings := []ec2.BlockDeviceMapping{}
	for attr, vals := range req.Form {
		if !strings.HasPrefix(attr, "BlockDeviceMapping.") {
			continue
		}
		fields := strings.SplitN(attr, ".", 3)
		if len(fields) < 3 || len(vals) != 1 {
			fatalf(400, "InvalidParameterValue", "bad param %q: %v", attr, vals)
		}
		index := atoi(fields[1]) - 1
		// Field name format: BlockDeviceMapping.<index>.<fieldName>....
		for len(mappings)-1 < index {
			mappings = append(mappings, ec2.BlockDeviceMapping{})
		}
		mapping := mappings[index]
		fieldName := fields[2]
		switch fieldName {
		case "DeviceName":
			mapping.DeviceName = vals[0]
		case "VirtualName":
			mapping.VirtualName = vals[0]
		case "Ebs.SnapshotId":
			mapping.SnapshotId = vals[0]
		case "Ebs.VolumeType":
			mapping.VolumeType = vals[0]
		case "Ebs.VolumeSize":
			mapping.VolumeSize = int64(atoi(vals[0]))
		case "Ebs.Iops":
			mapping.IOPS = int64(atoi(vals[0]))
		case "Ebs.DeleteOnTermination":
			val, err := strconv.ParseBool(vals[0])
			if err != nil {
				fatalf(400, "InvalidParameterValue", "bad flag %s: %s", fieldName, vals[0])
			}
			mapping.DeleteOnTermination = val
		default:
			fatalf(400, "InvalidParameterValue", "unknown field %s: %s", fieldName, vals[0])
		}
		mappings[index] = mapping
	}
	return mappings
}

func (srv *Server) createBlockDeviceMappingsOnRun(instId string, mappings []ec2.BlockDeviceMapping) []ec2.InstanceBlockDeviceMapping {
	results := make([]ec2.InstanceBlockDeviceMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.VirtualName != "" {
			// ephemeral block devices are attached, but do not
			// show up in block device mappings in responses.
			continue
		}
		results = append(results, ec2.InstanceBlockDeviceMapping{
			DeviceName:          mapping.DeviceName,
			VolumeId:            fmt.Sprintf("vol-%v", srv.volumeId.next()),
			AttachTime:          time.Now().Format(time.RFC3339),
			Status:              "attached",
			DeleteOnTermination: mapping.DeleteOnTermination,
		})
	}
	return results
}
