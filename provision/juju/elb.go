// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/flaviamissi/go-elb/aws"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/globocom/tsuru/log"
	"github.com/tsuru/config"
)

type elbInstance struct {
	id          string
	description string
	reasonCode  string
	state       string
	lb          string
}

func getELBEndpoint() *elb.ELB {
	access, err := config.GetString("aws:access-key-id")
	if err != nil {
		log.Fatal(err.Error())
	}
	secret, err := config.GetString("aws:secret-access-key")
	if err != nil {
		log.Fatal(err.Error())
	}
	endpoint, err := config.GetString("juju:elb-endpoint")
	if err != nil {
		log.Fatal(err.Error())
	}
	auth := aws.Auth{AccessKey: access, SecretKey: secret}
	region := aws.Region{ELBEndpoint: endpoint}
	return elb.New(auth, region)
}
