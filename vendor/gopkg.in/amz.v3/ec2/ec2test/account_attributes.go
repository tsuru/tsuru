//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011-2015 Canonical Ltd.
//
// This file contains code handling AWS account attributes
// discovery API.

package ec2test

import (
	"encoding/xml"
	"fmt"
	"net/http"

	"gopkg.in/amz.v3/ec2"
)

// SetInitialAttributes is deprecated and it's just an alias for
// SetAccountAttributes(). It's kept for now to prevent changing the
// ec2test public interface and should be removed in the next release.
func (srv *Server) SetInitialAttributes(attrs map[string][]string) error {
	return srv.SetAccountAttributes(attrs)
}

// SetAccountAttributes sets the given account attributes on the
// server. When the "default-vpc" attribute is specified, its value
// must match an existing VPC in the test server, otherwise it's an
// error. In addition, only the first value for "default-vpc", the
// rest (if any) are ignored.
func (srv *Server) SetAccountAttributes(attrs map[string][]string) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	for attrName, values := range attrs {
		srv.attributes[attrName] = values
		if attrName == "default-vpc" {
			if len(values) == 0 {
				return fmt.Errorf("no value(s) for attribute default-vpc")
			}
			defaultVPCId := values[0] // ignore the rest.
			if _, found := srv.vpcs[defaultVPCId]; !found {
				return fmt.Errorf("VPC %q not found", defaultVPCId)
			}
		}
	}
	return nil
}

func (srv *Server) accountAttributes(w http.ResponseWriter, req *http.Request, reqId string) interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	attrsMap := parseIDs(req.Form, "AttributeName.")
	var resp struct {
		XMLName xml.Name
		ec2.AccountAttributesResp
	}
	resp.XMLName = xml.Name{defaultXMLName, "DescribeAccountAttributesResponse"}
	resp.RequestId = reqId
	for attrName, _ := range attrsMap {
		vals, ok := srv.attributes[attrName]
		if !ok {
			fatalf(400, "InvalidParameterValue", "describe attrs: not found %q", attrName)
		}
		resp.Attributes = append(resp.Attributes, ec2.AccountAttribute{attrName, vals})
	}
	return &resp
}
