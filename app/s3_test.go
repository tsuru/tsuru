// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/api/bind"
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

func (s *S) TestCreateBucket(c *C) {
	app := App{Name: "myApp"}
	patchRandomReader()
	defer unpatchRandomReader()
	env, err := createBucket(&app)
	c.Assert(err, IsNil)
	defer func() {
		app.Env = map[string]bind.EnvVar{
			"TSURU_S3_ENDPOINT": {
				Name:         "TSURU_S3_ENDPOINT",
				Value:        env.endpoint,
				Public:       false,
				InstanceName: s3InstanceName,
			},
			"TSURU_S3_BUCKET": {
				Name:         "TSURU_S3_BUCKET",
				Value:        env.bucket,
				Public:       false,
				InstanceName: s3InstanceName,
			},
			"TSURU_S3_ACCESS_KEY_ID": {
				Name:         "TSURU_S3_ACCESS_KEY_ID",
				Value:        env.AccessKey,
				Public:       false,
				InstanceName: s3InstanceName,
			},
		}
		err := destroyBucket(&app)
		c.Assert(err, IsNil)
	}()
	c.Assert(env.bucket, HasLen, maxBucketSize)
	expected := "myappe3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3"
	c.Assert(env.bucket, Equals, expected)
	s3 := getS3Endpoint()
	_, err = s3.Bucket(expected).List("", "/", "", 100)
	c.Assert(err, IsNil)
	iam := getIAMEndpoint()
	resp, err := iam.GetUserPolicy("myapp", "app-myapp-bucket")
	c.Assert(err, IsNil)
	var policyBuffer bytes.Buffer
	policy.Execute(&policyBuffer, expected)
	c.Assert(resp.Policy.Document, Equals, policyBuffer.String())
}

// Issue 197.
func (s *S) TestCreateBucketIsAtomic(c *C) {
	app := App{Name: "myApp"}
	source := patchRandomReader()
	defer unpatchRandomReader()
	iamEndpoint := getIAMEndpoint()
	_, err := iamEndpoint.CreateUser("myapp", "/")
	c.Assert(err, IsNil)
	defer iamEndpoint.DeleteUser("myapp")
	env, err := createBucket(&app)
	c.Assert(err, NotNil)
	defer destroyBucket(&app)
	c.Assert(env, IsNil)
	_, err = iamEndpoint.GetUserPolicy("myapp", "app-myapp-bucket")
	c.Assert(err, NotNil)
	bucketName := fmt.Sprintf("myapp%x", source)
	bucket := getS3Endpoint().Bucket(bucketName)
	_, err = bucket.Get("non-existent")
	c.Assert(err, NotNil)
}

func (s *S) TestDestroyBucket(c *C) {
	dir, err := commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{
		Name:  "battery",
		Units: []Unit{{Machine: 1}},
	}
	bucket := fmt.Sprintf("battery%x", patchRandomReader())
	defer unpatchRandomReader()
	err = CreateApp(&app)
	c.Assert(err, IsNil)
	defer app.Destroy()
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
}
