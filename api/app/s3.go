// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/fsouza/go-iam"
	"github.com/globocom/config"
	"io"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/s3"
	"strings"
	"text/template"
)

type s3Env struct {
	aws.Auth
	bucket             string
	endpoint           string
	locationConstraint bool
}

func (s *s3Env) empty() bool {
	return s.bucket == "" || s.AccessKey == "" || s.SecretKey == ""
}

const (
	randBytes      = 32
	s3InstanceName = "tsurus3"
)

var (
	rReader = rand.Reader
	policy  = template.Must(template.New("policy").Parse(`{
  "Statement": [
    {
      "Action": [
        "s3:CreateBucket",
        "s3:DeleteBucket",
        "s3:DeleteBucketPolicy",
        "s3:DeleteBucketWebsite",
        "s3:PutBucketLogging",
        "s3:PutBucketPolicy",
        "s3:PutBucketRequestPayment",
        "s3:PutBucketVersioning",
        "s3:PutBucketWebsite"
      ],
      "Effect": "Deny",
      "Resource": [
        "arn:aws:s3:::{{.}}/*",
        "arn:aws:s3:::{{.}}"
      ]
    },
    {
      "Action": [
        "s3:*"
      ],
      "Effect": "Allow",
      "Resource": [
        "arn:aws:s3:::{{.}}/*",
        "arn:aws:s3:::{{.}}"
      ]
    }
  ]
}`))
)

func getAWSAuth() aws.Auth {
	access, err := config.GetString("aws:access-key-id")
	if err != nil {
		panic("FATAL: aws:access-key-id must be defined in configuration file.")
	}
	secret, err := config.GetString("aws:secret-access-key")
	if err != nil {
		panic("FATAL: aws:secret-access-key must be defined in configuration file.")
	}
	return aws.Auth{
		AccessKey: access,
		SecretKey: secret,
	}
}

func getS3Endpoint() *s3.S3 {
	regionName, _ := config.GetString("aws:s3:region-name")
	endpoint, err := config.GetString("aws:s3:endpoint")
	if err != nil {
		panic("FATAL: aws:s3:endpoint must be defined in configuration file.")
	}
	bucketEndpoint, _ := config.GetString("aws:s3:bucketEndpoint")
	locationConstraint, err := config.GetBool("aws:s3:location-constraint")
	if err != nil {
		panic("FATAL: aws:s3:location-constraint must be defined in configuration file.")
	}
	lowercaseBucket, err := config.GetBool("aws:s3:lowercase-bucket")
	if err != nil {
		panic("FATAL: aws:s3:lowercase-bucket must be defined in configuration file.")
	}
	region := aws.Region{
		Name:                 regionName,
		S3Endpoint:           endpoint,
		S3BucketEndpoint:     bucketEndpoint,
		S3LocationConstraint: locationConstraint,
		S3LowercaseBucket:    lowercaseBucket,
	}
	return s3.New(getAWSAuth(), region)
}

func getIAMEndpoint() *iam.IAM {
	endpoint, err := config.GetString("aws:iam:endpoint")
	if err != nil {
		panic("FATAL: aws:iam:endpoint must be defined in configuration file.")
	}
	region := aws.Region{IAMEndpoint: endpoint}
	return iam.New(getAWSAuth(), region)
}

func putBucket(appName string, bucketChan chan s3.Bucket, errChan chan error) {
	randPart := make([]byte, randBytes)
	n, err := rReader.Read(randPart)
	if err != nil {
		errChan <- err
		return
	}
	if n != randBytes {
		errChan <- io.ErrShortBuffer
		return
	}
	name := fmt.Sprintf("%s%x", appName, randPart)
	bucket := getS3Endpoint().Bucket(name)
	if err := bucket.PutBucket(s3.BucketOwnerFull); err != nil {
		errChan <- err
		return
	}
	bucketChan <- *bucket
}

func createIAMCredentials(appName string, keyChan chan iam.AccessKey, errChan chan error) {
	iamEndpoint := getIAMEndpoint()
	uResp, err := iamEndpoint.CreateUser(appName, fmt.Sprintf("/%s/", appName))
	if err != nil {
		errChan <- err
		return
	}
	kResp, err := iamEndpoint.CreateAccessKey(uResp.User.Name)
	if err != nil {
		errChan <- err
		return
	}
	keyChan <- kResp.AccessKey
}

func createBucket(app *App) (*s3Env, error) {
	var env s3Env
	appName := strings.ToLower(app.Name)
	errChan := make(chan error)
	bChan := make(chan s3.Bucket, 1)
	kChan := make(chan iam.AccessKey, 1)
	s := getS3Endpoint()
	go putBucket(appName, bChan, errChan)
	go createIAMCredentials(appName, kChan, errChan)
	iamEndpoint := getIAMEndpoint()
	var userName string
	for env.empty() {
		select {
		case k := <-kChan:
			env.AccessKey = k.Id
			env.SecretKey = k.Secret
			userName = k.UserName
		case bucket := <-bChan:
			env.bucket = bucket.Name
			env.locationConstraint = bucket.S3LocationConstraint
			env.endpoint = bucket.S3Endpoint
		case err := <-errChan:
			if env.bucket != "" {
				s.Bucket(env.bucket).DelBucket()
			}
			if userName != "" {
				iamEndpoint.DeleteUser(userName)
			}
			return nil, err
		}
	}
	policyName := fmt.Sprintf("app-%s-bucket", appName)
	var buf bytes.Buffer
	policy.Execute(&buf, env.bucket)
	if _, err := iamEndpoint.PutUserPolicy(userName, policyName, buf.String()); err != nil {
		return nil, err
	}
	return &env, nil
}

func destroyBucket(app *App) error {
	appName := strings.ToLower(app.Name)
	env := app.InstanceEnv(s3InstanceName)
	accessKeyId := env["TSURU_S3_ACCESS_KEY_ID"].Value
	bucketName := env["TSURU_S3_BUCKET"].Value
	policyName := fmt.Sprintf("app-%s-bucket", appName)
	s3Endpoint := getS3Endpoint()
	iamEndpoint := getIAMEndpoint()
	if _, err := iamEndpoint.DeleteUserPolicy(appName, policyName); err != nil {
		return err
	}
	bucket := s3Endpoint.Bucket(bucketName)
	if err := bucket.DelBucket(); err != nil {
		return err
	}
	if _, err := iamEndpoint.DeleteAccessKey(accessKeyId, appName); err != nil {
		return err
	}
	_, err := iamEndpoint.DeleteUser(appName)
	return err
}
