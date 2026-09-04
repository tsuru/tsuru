// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/pkg/errors"
	registrytest "github.com/tsuru/tsuru/registry/testing"
	check "gopkg.in/check.v1"
)

const testDigest = "sha256:ac9168d67991e02841c09fd1af9f41e0997571b32ad8f101813c7fa82f62f17f"

// fakeECRClient keeps images keyed by digest, each with the tags pointing at
// it, so tests can observe that deleting one tag removes the whole manifest.
type fakeECRClient struct {
	images map[string][]string

	describeInputs []*ecr.DescribeImagesInput
	describeOutput *ecr.DescribeImagesOutput
	describeErr    error

	deleteInputs []*ecr.BatchDeleteImageInput
	deleteOutput *ecr.BatchDeleteImageOutput
	deleteErr    error
}

func (f *fakeECRClient) tagsForDigest(digest string) []string {
	return f.images[digest]
}

func (f *fakeECRClient) DescribeImages(_ context.Context, in *ecr.DescribeImagesInput, _ ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error) {
	f.describeInputs = append(f.describeInputs, in)
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	if f.describeOutput != nil {
		return f.describeOutput, nil
	}
	tag := aws.ToString(in.ImageIds[0].ImageTag)
	for digest, tags := range f.images {
		if slices.Contains(tags, tag) {
			return &ecr.DescribeImagesOutput{ImageDetails: []ecrtypes.ImageDetail{{
				ImageDigest: aws.String(digest),
				ImageTags:   tags,
			}}}, nil
		}
	}
	return nil, &ecrtypes.ImageNotFoundException{Message: aws.String("image not found")}
}

func (f *fakeECRClient) BatchDeleteImage(_ context.Context, in *ecr.BatchDeleteImageInput, _ ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error) {
	f.deleteInputs = append(f.deleteInputs, in)
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	if f.deleteOutput != nil {
		return f.deleteOutput, nil
	}
	delete(f.images, aws.ToString(in.ImageIds[0].ImageDigest))
	return &ecr.BatchDeleteImageOutput{}, nil
}

type fakeECR struct {
	client   *fakeECRClient
	newCalls int
}

// setUpFakeECR replaces the ECR client factory with one handing out a fake
// holding a single image tagged both "v3" and "latest".
func setUpFakeECR() (*fakeECR, func()) {
	fake := &fakeECR{client: &fakeECRClient{
		images: map[string][]string{testDigest: {"v3", "latest"}},
	}}
	original := newECRClient
	newECRClient = func(context.Context, string) (ecrClient, error) {
		fake.newCalls++
		return fake.client, nil
	}
	return fake, func() {
		newECRClient = original
		ecrClients = map[string]ecrClient{}
	}
}

func (s *S) TestIsECRRegistry(c *check.C) {
	tests := []struct {
		host   string
		region string
		ok     bool
	}{
		{host: "123456789012.dkr.ecr.us-east-1.amazonaws.com", region: "us-east-1", ok: true},
		{host: "123456789012.dkr.ecr.eu-west-3.amazonaws.com", region: "eu-west-3", ok: true},
		{host: "123456789012.dkr.ecr-fips.us-gov-west-1.amazonaws.com", region: "us-gov-west-1", ok: true},
		{host: "123456789012.dkr.ecr.cn-north-1.amazonaws.com.cn", region: "cn-north-1", ok: true},
		{host: "123456789012.DKR.ECR.us-east-1.AMAZONAWS.COM", region: "us-east-1", ok: true},
		{host: "123456789012.dkr.ecr.us-east-1.amazonaws.com.", region: "us-east-1", ok: true},
		{host: "12345.dkr.ecr.us-east-1.amazonaws.com"},
		{host: "123456789012.dkr.ecr.us-east-1.amazonaws.com.evil.io"},
		{host: "notecr.123456789012.dkr.ecr.us-east-1.amazonaws.com"},
		{host: "123456789012.dkr.ecr.us-east-1.amazonaws.com:443"},
		{host: "registry.hub.docker.com"},
		{host: "gcr.io"},
		{host: "myregistry:5000"},
	}
	for _, tt := range tests {
		region, ok := isECRRegistry(tt.host)
		c.Check(ok, check.Equals, tt.ok, check.Commentf("host: %s", tt.host))
		c.Check(region, check.Equals, tt.region, check.Commentf("host: %s", tt.host))
	}
}

// A tag must be resolved to its digest and the manifest deleted by digest:
// deleting by tag would only remove the tag and keep the image whenever
// another tag, such as "latest", still points at the same manifest.
func (s *S) TestRemoveImageOnECRDeletesManifestWithAllItsTags(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	c.Assert(fake.client.tagsForDigest(testDigest), check.DeepEquals, []string{"v3", "latest"})

	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)

	c.Assert(fake.client.describeInputs, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.describeInputs[0].RepositoryName), check.Equals, "tsuru/app-myapp")
	c.Assert(fake.client.describeInputs[0].ImageIds, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.describeInputs[0].ImageIds[0].ImageTag), check.Equals, "v3")

	c.Assert(fake.client.deleteInputs, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.deleteInputs[0].RepositoryName), check.Equals, "tsuru/app-myapp")
	c.Assert(fake.client.deleteInputs[0].ImageIds, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.deleteInputs[0].ImageIds[0].ImageDigest), check.Equals, testDigest)
	c.Assert(fake.client.deleteInputs[0].ImageIds[0].ImageTag, check.IsNil)

	// The manifest is gone, so "latest" no longer points at anything.
	c.Assert(fake.client.tagsForDigest(testDigest), check.IsNil)
}

func (s *S) TestRemoveImageOnECRNestedRepository(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/team/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	c.Assert(aws.ToString(fake.client.describeInputs[0].RepositoryName), check.Equals, "team/tsuru/app-myapp")
	c.Assert(fake.client.deleteInputs, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.deleteInputs[0].RepositoryName), check.Equals, "team/tsuru/app-myapp")
	c.Assert(aws.ToString(fake.client.deleteInputs[0].ImageIds[0].ImageDigest), check.Equals, testDigest)
}

func (s *S) TestRemoveImageOnECRTagNotFound(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v0")
	c.Assert(errors.Cause(err), check.Equals, ErrImageNotFound)
	c.Assert(fake.client.deleteInputs, check.HasLen, 0)
	err = RemoveImageIgnoreNotFound(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v0")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveImageOnECRTagWithoutImageDetails(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	fake.client.describeOutput = &ecr.DescribeImagesOutput{}
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(errors.Cause(err), check.Equals, ErrImageNotFound)
	c.Assert(fake.client.deleteInputs, check.HasLen, 0)
}

func (s *S) TestRemoveImageOnECRRepositoryNotFound(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	fake.client.describeErr = &ecrtypes.RepositoryNotFoundException{Message: aws.String("repository not found")}
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(errors.Cause(err), check.Equals, ErrImageNotFound)
	err = RemoveImageIgnoreNotFound(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
}

// Anything other than a missing image must be reported instead of being
// mistaken for an already deleted image, otherwise garbage collection would
// drop the version from storage and orphan the manifest.
func (s *S) TestRemoveImageOnECRResolveErrorIsNotTreatedAsNotFound(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	fake.client.describeErr = &ecrtypes.InvalidParameterException{Message: aws.String("invalid tag")}
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.ErrorMatches, `failed to get digest for image tsuru/app-myapp:v3 on ECR: .*invalid tag`)
	c.Assert(errors.Cause(err), check.Not(check.Equals), ErrImageNotFound)
	c.Assert(fake.client.deleteInputs, check.HasLen, 0)
	err = RemoveImageIgnoreNotFound(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.NotNil)
}

func (s *S) TestRemoveImageOnECRResolveAPIError(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	fake.client.describeErr = errors.New("access denied")
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.ErrorMatches, `failed to get digest for image tsuru/app-myapp:v3 on ECR: access denied`)
	c.Assert(fake.client.deleteInputs, check.HasLen, 0)
}

func (s *S) TestRemoveImageOnECRImageNotFoundOnDelete(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	fake.client.deleteOutput = &ecr.BatchDeleteImageOutput{Failures: []ecrtypes.ImageFailure{{
		FailureCode:   ecrtypes.ImageFailureCodeImageNotFound,
		FailureReason: aws.String("Requested image not found"),
	}}}
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(errors.Cause(err), check.Equals, ErrImageNotFound)
	err = RemoveImageIgnoreNotFound(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveImageOnECRImageFailure(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	fake.client.deleteOutput = &ecr.BatchDeleteImageOutput{Failures: []ecrtypes.ImageFailure{{
		FailureCode:   ecrtypes.ImageFailureCodeInvalidImageDigest,
		FailureReason: aws.String("invalid digest"),
	}}}
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.ErrorMatches, `failed to remove image tsuru/app-myapp:v3 on ECR: InvalidImageDigest: invalid digest`)
}

func (s *S) TestRemoveImageOnECRDeleteAPIError(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	fake.client.deleteErr = errors.New("access denied")
	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.ErrorMatches, `failed to remove image tsuru/app-myapp:v3 on ECR: access denied`)
}

func (s *S) TestRemoveImageOnOtherRegistryDoesNotUseECR(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	s.server.SetTokenAuth(false, false)
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-myapp", Tags: map[string]string{"v1": "abcdefg"}})
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	c.Assert(fake.newCalls, check.Equals, 0)
	c.Assert(fake.client.describeInputs, check.HasLen, 0)
	c.Assert(fake.client.deleteInputs, check.HasLen, 0)
}
