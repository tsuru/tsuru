// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/fsouza/go-iam"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/s3"
	. "launchpad.net/gocheck"
)

func patchRandomReader() []byte {
	source := make([]byte, maxBucketSize)
	for i := 0; i < maxBucketSize; i++ {
		source[i] = 0xe3
	}
	rReader = bytes.NewReader(source)
	return source
}

func unpatchRandomReader() {
	rReader = rand.Reader
}

func (s *S) TestGetAWSAuth(c *C) {
	access, err := config.Get("aws:access-key-id")
	c.Assert(err, IsNil)
	secret, err := config.Get("aws:secret-access-key")
	c.Assert(err, IsNil)
	auth := getAWSAuth()
	c.Assert(auth.AccessKey, Equals, access)
	c.Assert(auth.SecretKey, Equals, secret)
}

func (s *S) TestGetS3Endpoint(c *C) {
	oldRegion, _ := config.Get("aws:s3:region-name")
	defer config.Set("aws:s3:region-name", oldRegion)
	config.Set("aws:s3:region-name", "myregion")
	edp, err := config.GetString("aws:s3:endpoint")
	c.Assert(err, IsNil)
	locConst, err := config.GetBool("aws:s3:location-constraint")
	c.Assert(err, IsNil)
	lwrCaseBucket, err := config.GetBool("aws:s3:lowercase-bucket")
	c.Assert(err, IsNil)
	s3 := getS3Endpoint()
	c.Assert(s3.S3Endpoint, Equals, edp)
	c.Assert(s3.S3LocationConstraint, Equals, locConst)
	c.Assert(s3.S3LowercaseBucket, Equals, lwrCaseBucket)
	c.Assert(s3.Region.Name, Equals, "myregion")
}

func (s *S) TestGetIAMEndpoint(c *C) {
	edp, err := config.GetString("aws:iam:endpoint")
	c.Assert(err, IsNil)
	iam := getIAMEndpoint()
	c.Assert(iam.IAMEndpoint, Equals, edp)
}

func (s *S) TestGetIAMEndpointDefault(c *C) {
	defaultEndpoint := "https://iam.amazonaws.com/"
	old, err := config.GetString("aws:iam:endpoint")
	c.Assert(err, IsNil)
	config.Unset("aws:iam:endpoint")
	defer config.Set("aws:iam:endpoint", old)
	iam := getIAMEndpoint()
	c.Assert(iam.IAMEndpoint, Equals, defaultEndpoint)
}

func (s *S) TestPutBucket(c *C) {
	patchRandomReader()
	defer unpatchRandomReader()
	bucket, err := putBucket("holysky")
	c.Assert(err, IsNil)
	c.Assert(bucket.Name, Equals, "holyskye3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3")
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{S3Endpoint: s.t.S3Server.URL()}
	s3client := s3.New(auth, region)
	_, err = s3client.Bucket(bucket.Name).List("", "/", "", 100)
	c.Assert(err, IsNil)
	defer s3client.Bucket(bucket.Name).DelBucket()
}

func (s *S) TestCreateIAMUser(c *C) {
	user, err := createIAMUser("rules")
	c.Assert(err, IsNil)
	c.Assert(user.Name, Equals, "rules")
	c.Assert(user.Path, Equals, "/rules/")
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	_, err = iamClient.GetUser(user.Name)
	defer iamClient.DeleteUser(user.Name)
	c.Assert(err, IsNil)
}

func (s *S) TestCreateIAMAccessKey(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	user, err := createIAMUser("hit")
	c.Assert(err, IsNil)
	defer iamClient.DeleteUser(user.Name)
	key, err := createIAMAccessKey(user)
	c.Assert(err, IsNil)
	c.Assert(key.Id, Not(Equals), "")
	c.Assert(key.Secret, Not(Equals), "")
	c.Assert(key.UserName, Equals, user.Name)
}

func (s *S) TestCreateIAMUserPolicy(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	user, err := createIAMUser("fight")
	c.Assert(err, IsNil)
	defer iamClient.DeleteUser(user.Name)
	userPolicy, err := createIAMUserPolicy(user, "fight", "mybucket")
	c.Assert(err, IsNil)
	c.Assert(userPolicy.UserName, Equals, user.Name)
	c.Assert(userPolicy.Name, Equals, "app-fight-bucket")
	resp, err := iamClient.GetUserPolicy(user.Name, userPolicy.Name)
	c.Assert(err, IsNil)
	var buf bytes.Buffer
	policy.Execute(&buf, "mybucket")
	c.Assert(resp.Policy.Document, Equals, buf.String())
}

func (s *S) TestDestroyBucket(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{
		Name:  "battery",
		Units: []Unit{{Machine: 1}},
	}
	bucket := fmt.Sprintf("battery%x", patchRandomReader())
	defer unpatchRandomReader()
	err := CreateApp(&app, 1, []auth.Team{s.team})
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	defer s.provisioner.Destroy(&app)
	app.Get()
	err = destroyBucket(&app)
	c.Assert(err, IsNil)
	s3 := getS3Endpoint()
	_, err = s3.Bucket(bucket).List("", "/", "", 100)
	c.Assert(err, NotNil)
	iam := getIAMEndpoint()
	_, err = iam.GetUserPolicy("app-battery-bucket", "battery")
	c.Assert(err, NotNil)
	_, err = iam.DeleteAccessKey(app.Env["TSURU_S3_ACCESS_KEY_ID"].Value, "")
	c.Assert(err, NotNil)
	msg, err := aqueue().Get(1e6)
	c.Assert(err, IsNil)
	c.Assert(msg.Args, DeepEquals, []string{app.Name})
	msg.Delete()
}
