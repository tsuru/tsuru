package app

import (
	"bytes"
	"crypto/rand"
	"fmt"
	. "launchpad.net/gocheck"
)

func patchRandomReader() []byte {
	source := make([]byte, randBytes)
	for i := 0; i < randBytes; i++ {
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
	source := patchRandomReader()
	defer unpatchRandomReader()
	env, err := createBucket(&app)
	c.Assert(err, IsNil)
	expected := fmt.Sprintf("myapp%x", source)
	c.Assert(env.bucket, Equals, expected)
	s3 := getS3Endpoint()
	_, err = s3.Bucket(expected).List("", "/", "", 100)
	c.Assert(err, IsNil)
	iam := getIAMEndpoint()
	resp, err := iam.GetUserPolicy("app-myapp-bucket", "myapp")
	c.Assert(err, IsNil)
	var policyBuffer bytes.Buffer
	policy.Execute(&policyBuffer, expected)
	c.Assert(resp.Policy.Document, Equals, policyBuffer.String())
}
