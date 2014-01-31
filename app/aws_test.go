// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/iam"
	"launchpad.net/goamz/s3"
	"launchpad.net/gocheck"
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

func (s *S) TestGetAWSAuth(c *gocheck.C) {
	access, err := config.Get("aws:access-key-id")
	c.Assert(err, gocheck.IsNil)
	secret, err := config.Get("aws:secret-access-key")
	c.Assert(err, gocheck.IsNil)
	auth := getAWSAuth()
	c.Assert(auth.AccessKey, gocheck.Equals, access)
	c.Assert(auth.SecretKey, gocheck.Equals, secret)
}

func (s *S) TestGetS3Endpoint(c *gocheck.C) {
	oldRegion, _ := config.Get("aws:s3:region-name")
	defer config.Set("aws:s3:region-name", oldRegion)
	config.Set("aws:s3:region-name", "myregion")
	edp, err := config.GetString("aws:s3:endpoint")
	c.Assert(err, gocheck.IsNil)
	locConst, err := config.GetBool("aws:s3:location-constraint")
	c.Assert(err, gocheck.IsNil)
	lwrCaseBucket, err := config.GetBool("aws:s3:lowercase-bucket")
	c.Assert(err, gocheck.IsNil)
	s3 := getS3Endpoint()
	c.Assert(s3.S3Endpoint, gocheck.Equals, edp)
	c.Assert(s3.S3LocationConstraint, gocheck.Equals, locConst)
	c.Assert(s3.S3LowercaseBucket, gocheck.Equals, lwrCaseBucket)
	c.Assert(s3.Region.Name, gocheck.Equals, "myregion")
}

func (s *S) TestGetIAMEndpoint(c *gocheck.C) {
	edp, err := config.GetString("aws:iam:endpoint")
	c.Assert(err, gocheck.IsNil)
	iam := getIAMEndpoint()
	c.Assert(iam.IAMEndpoint, gocheck.Equals, edp)
}

func (s *S) TestGetIAMEndpointDefault(c *gocheck.C) {
	defaultEndpoint := "https://iam.amazonaws.com/"
	old, err := config.GetString("aws:iam:endpoint")
	c.Assert(err, gocheck.IsNil)
	config.Unset("aws:iam:endpoint")
	defer config.Set("aws:iam:endpoint", old)
	iam := getIAMEndpoint()
	c.Assert(iam.IAMEndpoint, gocheck.Equals, defaultEndpoint)
}

func (s *S) TestPutBucket(c *gocheck.C) {
	patchRandomReader()
	defer unpatchRandomReader()
	bucket, err := putBucket("holysky")
	c.Assert(err, gocheck.IsNil)
	c.Assert(bucket.Name, gocheck.Equals, "holyskye3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3")
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{S3Endpoint: s.t.S3Server.URL()}
	s3client := s3.New(auth, region)
	_, err = s3client.Bucket(bucket.Name).List("", "/", "", 100)
	c.Assert(err, gocheck.IsNil)
	defer s3client.Bucket(bucket.Name).DelBucket()
}

func (s *S) TestCreateIAMUser(c *gocheck.C) {
	user, err := createIAMUser("rules")
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Name, gocheck.Equals, "rules")
	c.Assert(user.Path, gocheck.Equals, "/rules/")
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	_, err = iamClient.GetUser(user.Name)
	defer iamClient.DeleteUser(user.Name)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCreateIAMAccessKey(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	user, err := createIAMUser("hit")
	c.Assert(err, gocheck.IsNil)
	defer iamClient.DeleteUser(user.Name)
	key, err := createIAMAccessKey(user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(key.Id, gocheck.Not(gocheck.Equals), "")
	c.Assert(key.Secret, gocheck.Equals, "")
	c.Assert(key.UserName, gocheck.Equals, user.Name)
}

func (s *S) TestCreateIAMUserPolicy(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	user, err := createIAMUser("fight")
	c.Assert(err, gocheck.IsNil)
	defer iamClient.DeleteUser(user.Name)
	userPolicy, err := createIAMUserPolicy(user, "fight", "mybucket")
	c.Assert(err, gocheck.IsNil)
	c.Assert(userPolicy.UserName, gocheck.Equals, user.Name)
	c.Assert(userPolicy.Name, gocheck.Equals, "app-fight-bucket")
	resp, err := iamClient.GetUserPolicy(user.Name, userPolicy.Name)
	c.Assert(err, gocheck.IsNil)
	var buf bytes.Buffer
	policy.Execute(&buf, "mybucket")
	c.Assert(resp.Policy.Document, gocheck.Equals, buf.String())
}

func (s *S) TestDestroyBucket(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{
		Name:     "battery",
		Platform: "python",
		Units:    []Unit{{Machine: 1}},
	}
	bucket := fmt.Sprintf("battery%x", patchRandomReader())
	defer unpatchRandomReader()
	err := CreateApp(&app, s.user)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	defer s.provisioner.Destroy(&app)
	otherApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	err = destroyBucket(otherApp)
	c.Assert(err, gocheck.IsNil)
	s3 := getS3Endpoint()
	_, err = s3.Bucket(bucket).List("", "/", "", 100)
	c.Assert(err, gocheck.NotNil)
	iam := getIAMEndpoint()
	_, err = iam.GetUserPolicy("app-battery-bucket", "battery")
	c.Assert(err, gocheck.NotNil)
	_, err = iam.DeleteAccessKey(otherApp.Env["TSURU_S3_ACCESS_KEY_ID"].Value, "")
	c.Assert(err, gocheck.NotNil)
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{otherApp.Name})
}
