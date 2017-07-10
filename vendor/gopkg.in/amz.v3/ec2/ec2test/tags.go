// The ec2test package implements a fake EC2 provider with
// the capability of inducing errors on any given operation,
// and retrospectively determining what operations have been
// carried out.
//
// This file contains code handling AWS API for tagging resources.
package ec2test

import (
	"encoding/xml"
	"net/http"
	"strings"

	"gopkg.in/amz.v3/ec2"
)

func (srv *Server) createTags(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	var resourceIds []string
	var tags []ec2.Tag

	for attr, vals := range req.Form {
		if strings.HasPrefix(attr, "ResourceId.") {
			fields := strings.SplitN(attr, ".", 2)
			if len(fields) != 2 {
				fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
			}
			i := atoi(fields[1])
			for i > len(resourceIds) {
				resourceIds = append(resourceIds, "")
			}
			resourceIds[i-1] = vals[0]
		} else if strings.HasPrefix(attr, "Tag.") {
			fields := strings.SplitN(attr, ".", 3)
			if len(fields) != 3 {
				fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
			}
			i := atoi(fields[1])
			for i > len(tags) {
				tags = append(tags, ec2.Tag{})
			}
			switch fields[2] {
			case "Key":
				tags[i-1].Key = vals[0]
			case "Value":
				tags[i-1].Value = vals[0]
			default:
				fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
			}
		} else {
			switch attr {
			case "AWSAccessKeyId", "Action", "Signature", "SignatureMethod", "SignatureVersion",
				"Version", "Timestamp":
				continue
			default:
				fatalf(400, "InvalidParameterValue", "unknown field %s: %s", attr, vals[0])
			}
		}
	}

	// Each resource can have a maximum of 10 tags.
	const tagLimit = 10
	for _, resourceId := range resourceIds {
		resourceTags := srv.tags(resourceId)
		for _, tag := range tags {
			var found bool
			for i := range *resourceTags {
				if (*resourceTags)[i].Key != tag.Key {
					continue
				}
				(*resourceTags)[i].Value = tag.Value
				found = true
				break
			}
			if found {
				continue
			}
			if len(*resourceTags) == tagLimit {
				fatalf(400, "TagLimitExceeded", "The maximum number of Tags for a resource has been reached.")
			}
			*resourceTags = append(*resourceTags, tag)
		}
	}

	return &ec2.SimpleResp{
		XMLName:   xml.Name{defaultXMLName, "CreateTagsResponse"},
		RequestId: reqId,
		Return:    true,
	}
}

func (srv *Server) tags(id string) *[]ec2.Tag {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) == 0 {
		fatalf(400, "InvalidID", "The ID '%s' is not valid", id)
	}
	switch parts[0] {
	case "i":
		if inst, ok := srv.instances[id]; ok {
			return &inst.tags
		}
	case "sg":
		if group, ok := srv.groups[id]; ok {
			return &group.tags
		}
	case "vol":
		if vol, ok := srv.volumes[id]; ok {
			return &vol.Tags
		}
		// TODO(axw) more resources as necessary.
	}
	fatalf(400, "InvalidID", "The ID '%s' is not valid", id)
	return nil
}

func matchTag(tags []ec2.Tag, key, value string) bool {
	for _, tag := range tags {
		if tag.Key == key {
			return tag.Value == value
		}
	}
	return false
}
