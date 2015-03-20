// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	"gopkg.in/amz.v2/aws"
	"gopkg.in/amz.v2/ec2"
	"gopkg.in/amz.v2/ec2/ec2test"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	srv    *ec2test.Server
	region aws.Region
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	var err error
	s.srv, err = ec2test.NewServer()
	c.Assert(err, check.IsNil)
	s.region = aws.Region{
		Name:        "myregion",
		EC2Endpoint: s.srv.URL(),
		Sign:        aws.SignV2,
	}
	aws.Regions["myregion"] = s.region
	config.Set("iaas:ec2:key-id", "mykey")
	config.Set("iaas:ec2:secret-key", "mysecret")
}

func (s *S) TearDownTest(c *check.C) {
	s.srv.Quit()
}

func (s *S) TestCreateEC2Handler(c *check.C) {
	ec2iaas := NewEC2IaaS()
	handler, err := ec2iaas.createEC2Handler(aws.APNortheast)
	c.Assert(err, check.IsNil)
	c.Assert(handler.Region.EC2Endpoint, check.DeepEquals, aws.APNortheast.EC2Endpoint)
	c.Assert(handler.Auth.AccessKey, check.Equals, "mykey")
	c.Assert(handler.Auth.SecretKey, check.Equals, "mysecret")
}

func (s *S) TestCreateMachine(c *check.C) {
	params := map[string]string{
		"region": "myregion",
		"image":  "ami-xxxxxx",
		"type":   "m1.micro",
	}
	ec2iaas := NewEC2IaaS()
	m, err := ec2iaas.CreateMachine(params)
	m.CreationParams = map[string]string{"region": "myregion"}
	defer ec2iaas.DeleteMachine(m)
	c.Assert(err, check.IsNil)
	c.Assert(m.Id, check.Matches, `i-\d`)
	c.Assert(m.Address, check.Matches, `i-\d.testing.invalid`)
	c.Assert(m.Status, check.Equals, "pending")
}

func (s *S) TestCreateMachineTimeoutError(c *check.C) {
	config.Set("iaas:ec2:wait-timeout", "1")
	defer config.Unset("iaas:ec2:wait-timeout")
	var calledActions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("Action")
		calledActions = append(calledActions, action)
		if action == "DescribeInstances" {
			w.Write([]byte(`
<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2015-10-01/">
<requestId>xxx</requestId>
<reservationSet>
      <item>
        <reservationId>r-1</reservationId>
        <instancesSet>
          <item>
            <instanceId>i-1</instanceId>
          </item>
        </instancesSet>
      </item>
</reservationSet>
</DescribeInstancesResponse>`))
			return
		}
		req, err := http.NewRequest(r.Method, s.srv.URL()+r.RequestURI, r.Body)
		c.Assert(err, check.IsNil)
		rsp, err := http.DefaultClient.Do(req)
		c.Assert(err, check.IsNil)
		w.WriteHeader(rsp.StatusCode)
		bytes, err := ioutil.ReadAll(rsp.Body)
		if action == "RunInstances" {
			re := regexp.MustCompile(`<dnsName>.+</dnsName>`)
			bytes = re.ReplaceAll(bytes, []byte{})
		}
		c.Assert(err, check.IsNil)
		w.Write(bytes)
	}))
	timeoutRegion := aws.Region{
		Name:        "timeoutregion",
		EC2Endpoint: server.URL,
		Sign:        aws.SignV2,
	}
	aws.Regions["timeoutregion"] = timeoutRegion
	params := map[string]string{
		"region": "timeoutregion",
		"image":  "ami-xxxxxx",
		"type":   "m1.micro",
	}
	ec2iaas := NewEC2IaaS()
	_, err := ec2iaas.CreateMachine(params)
	c.Assert(err, check.ErrorMatches, `ec2: time out after .+? waiting for instance .+? to start`)
	c.Assert(calledActions[len(calledActions)-1], check.Equals, "TerminateInstances")
}

func (s *S) TestCreateMachineDefaultRegion(c *check.C) {
	defaultRegionServer, err := ec2test.NewServer()
	c.Assert(err, check.IsNil)
	region := aws.Region{
		Name:        defaultRegion,
		EC2Endpoint: defaultRegionServer.URL(),
		Sign:        aws.SignV2,
	}
	aws.Regions[defaultRegion] = region
	params := map[string]string{
		"image": "ami-xxxxxx",
		"type":  "m1.micro",
	}
	expectedParams := map[string]string{
		"image":  "ami-xxxxxx",
		"type":   "m1.micro",
		"region": defaultRegion,
	}
	ec2iaas := NewEC2IaaS()
	m, err := ec2iaas.CreateMachine(params)
	c.Assert(err, check.IsNil)
	c.Assert(params, check.DeepEquals, expectedParams)
	m.CreationParams = params
	defer ec2iaas.DeleteMachine(m)
	c.Assert(m.Id, check.Matches, `i-\d`)
	c.Assert(m.Address, check.Matches, `i-\d.testing.invalid`)
	c.Assert(m.Status, check.Equals, "pending")
}

func (s *S) TestWaitForDnsName(c *check.C) {
	ec2iaas := NewEC2IaaS()
	handler, err := ec2iaas.createEC2Handler(s.region)
	c.Assert(err, check.IsNil)
	options := ec2.RunInstances{
		ImageId:      "ami-xxx",
		InstanceType: "m1.small",
		MinCount:     1,
		MaxCount:     1,
	}
	resp, err := handler.RunInstances(&options)
	c.Assert(err, check.IsNil)
	instance := &resp.Instances[0]
	instance.DNSName = ""
	instance, err = ec2iaas.waitForDnsName(handler, instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.DNSName, check.Matches, `i-\d.testing.invalid`)
}

func (s *S) TestCreateMachineValidations(c *check.C) {
	ec2iaas := NewEC2IaaS()
	_, err := ec2iaas.CreateMachine(map[string]string{
		"region": "invalid-region",
	})
	c.Assert(err, check.ErrorMatches, `region "invalid-region" not found`)
	_, err = ec2iaas.CreateMachine(map[string]string{
		"region": "myregion",
	})
	c.Assert(err, check.ErrorMatches, "image param required")
	_, err = ec2iaas.CreateMachine(map[string]string{
		"region": "myregion",
		"image":  "ami-xxxxx",
	})
	c.Assert(err, check.ErrorMatches, "type param required")
}

func (s *S) TestDeleteMachine(c *check.C) {
	insts := s.srv.NewInstances(1, "m1.small", "ami-x", ec2.InstanceState{}, nil)
	m := iaas.Machine{
		Id:             insts[0],
		CreationParams: map[string]string{"region": "myregion"},
	}
	ec2iaas := NewEC2IaaS()
	err := ec2iaas.DeleteMachine(&m)
	c.Assert(err, check.IsNil)
}

func (s *S) TestDeleteMachineValidations(c *check.C) {
	insts := s.srv.NewInstances(1, "m1.small", "ami-x", ec2.InstanceState{}, nil)
	ec2iaas := NewEC2IaaS()
	m := &iaas.Machine{
		Id:             insts[0],
		CreationParams: map[string]string{"region": "invalid"},
	}
	err := ec2iaas.DeleteMachine(m)
	c.Assert(err, check.ErrorMatches, `region "invalid" not found`)
	m = &iaas.Machine{
		Id:             insts[0],
		CreationParams: map[string]string{},
	}
	err = ec2iaas.DeleteMachine(m)
	c.Assert(err, check.ErrorMatches, `region creation param required`)
}

func (s *S) TestClone(c *check.C) {
	iaas := NewEC2IaaS()
	clonned := iaas.Clone("something")
	c.Assert(clonned, check.FitsTypeOf, iaas)
	clonnedIaas, _ := clonned.(*EC2IaaS)
	c.Assert(iaas.base.IaaSName, check.Equals, "")
	c.Assert(clonnedIaas.base.IaaSName, check.Equals, "something")
}
