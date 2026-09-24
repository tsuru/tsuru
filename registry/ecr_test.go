// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
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

	newCallHosts   []string
	newCallRegions []string
	newCallFIPS    []bool
}

// setUpFakeECR replaces the ECR client factory with one handing out a fake
// holding a single image tagged both "v3" and "latest".
func setUpFakeECR() (*fakeECR, func()) {
	fake := &fakeECR{client: &fakeECRClient{
		images: map[string][]string{testDigest: {"v3", "latest"}},
	}}
	original := newECRClient
	newECRClient = func(_ context.Context, host, region string, fips bool) (ecrClient, error) {
		fake.newCalls++
		fake.newCallHosts = append(fake.newCallHosts, host)
		fake.newCallRegions = append(fake.newCallRegions, region)
		fake.newCallFIPS = append(fake.newCallFIPS, fips)
		return fake.client, nil
	}
	return fake, func() {
		newECRClient = original
		ecrClientMu.Lock()
		ecrClients = map[string]ecrClient{}
		ecrClientMu.Unlock()
	}
}

func (s *S) TestIsECRRegistry(c *check.C) {
	tests := []struct {
		host      string
		accountID string
		region    string
		fips      bool
		ok        bool
	}{
		{host: "123456789012.dkr.ecr.us-east-1.amazonaws.com", accountID: "123456789012", region: "us-east-1", ok: true},
		{host: "123456789012.dkr.ecr.eu-west-3.amazonaws.com", accountID: "123456789012", region: "eu-west-3", ok: true},
		{host: "123456789012.dkr.ecr-fips.us-gov-west-1.amazonaws.com", accountID: "123456789012", region: "us-gov-west-1", fips: true, ok: true},
		{host: "123456789012.dkr.ecr.cn-north-1.amazonaws.com.cn", accountID: "123456789012", region: "cn-north-1", ok: true},
		{host: "999999999999.DKR.ECR.us-east-1.AMAZONAWS.COM", accountID: "999999999999", region: "us-east-1", ok: true},
		{host: "123456789012.dkr.ecr.us-east-1.amazonaws.com.", accountID: "123456789012", region: "us-east-1", ok: true},
		{host: "12345.dkr.ecr.us-east-1.amazonaws.com"},
		{host: "123456789012.dkr.ecr.us-east-1.amazonaws.com.evil.io"},
		{host: "notecr.123456789012.dkr.ecr.us-east-1.amazonaws.com"},
		{host: "123456789012.dkr.ecr.us-east-1.amazonaws.com:443"},
		{host: "registry.hub.docker.com"},
		{host: "gcr.io"},
		{host: "myregistry:5000"},
	}
	for _, tt := range tests {
		accountID, region, fips, ok := isECRRegistry(tt.host)
		c.Check(ok, check.Equals, tt.ok, check.Commentf("host: %s", tt.host))
		c.Check(accountID, check.Equals, tt.accountID, check.Commentf("host: %s", tt.host))
		c.Check(region, check.Equals, tt.region, check.Commentf("host: %s", tt.host))
		c.Check(fips, check.Equals, tt.fips, check.Commentf("host: %s", tt.host))
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
	c.Assert(aws.ToString(fake.client.describeInputs[0].RegistryId), check.Equals, "123456789012")
	c.Assert(aws.ToString(fake.client.describeInputs[0].RepositoryName), check.Equals, "tsuru/app-myapp")
	c.Assert(fake.client.describeInputs[0].ImageIds, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.describeInputs[0].ImageIds[0].ImageTag), check.Equals, "v3")

	c.Assert(fake.client.deleteInputs, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.deleteInputs[0].RegistryId), check.Equals, "123456789012")
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

// The account ID embedded in the registry host is the account the image
// actually lives in and must be threaded through as RegistryId on every
// call: omitting it makes the SDK default to the signing identity's own
// account, which is wrong for a repository accessed cross-account.
func (s *S) TestRemoveImageOnECRUsesAccountIDFromHostAsRegistryID(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()
	err := RemoveImage(context.TODO(), "999999999999.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	c.Assert(fake.client.describeInputs, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.describeInputs[0].RegistryId), check.Equals, "999999999999")
	c.Assert(fake.client.deleteInputs, check.HasLen, 1)
	c.Assert(aws.ToString(fake.client.deleteInputs[0].RegistryId), check.Equals, "999999999999")
}

// A repository accessed through the FIPS host variant must use the FIPS API
// endpoint, and must not share a cached client with the standard endpoint
// for the same region: the two are different endpoints even though they
// resolve to the same region string.
func (s *S) TestRemoveImageOnECRFIPSHostUsesFIPSEndpointAndSeparateCache(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()

	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	fake.client.images[testDigest] = []string{"v3", "latest"} // put the image back for the next call

	err = RemoveImage(context.TODO(), "123456789012.dkr.ecr-fips.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)

	c.Assert(fake.newCalls, check.Equals, 2)
	c.Assert(fake.newCallHosts, check.DeepEquals, []string{
		"123456789012.dkr.ecr.us-east-1.amazonaws.com",
		"123456789012.dkr.ecr-fips.us-east-1.amazonaws.com",
	})
	c.Assert(fake.newCallRegions, check.DeepEquals, []string{"us-east-1", "us-east-1"})
	c.Assert(fake.newCallFIPS, check.DeepEquals, []bool{false, true})
}

// Two different accounts in the same region must not share a cached client
// either, since each host may carry its own proxy:<host> configuration.
func (s *S) TestRemoveImageOnECRDifferentAccountsGetSeparateCachedClients(c *check.C) {
	fake, teardown := setUpFakeECR()
	defer teardown()

	err := RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	fake.client.images[testDigest] = []string{"v3", "latest"} // put the image back for the next call

	err = RemoveImage(context.TODO(), "999999999999.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)

	c.Assert(fake.newCalls, check.Equals, 2)
	c.Assert(fake.newCallHosts, check.DeepEquals, []string{
		"123456789012.dkr.ecr.us-east-1.amazonaws.com",
		"999999999999.dkr.ecr.us-east-1.amazonaws.com",
	})

	// A second call against the same host as the first reuses its client.
	fake.client.images[testDigest] = []string{"v3", "latest"}
	err = RemoveImage(context.TODO(), "123456789012.dkr.ecr.us-east-1.amazonaws.com/tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	c.Assert(fake.newCalls, check.Equals, 2)
}

// ecrHTTPClient is what actually issues ECR API requests, so it must route
// through the same proxy:<host> config entry the Distribution path honors,
// with the destination keyed by the ECR host, not the AWS API host.
func (s *S) TestECRHTTPClientHonorsProxyConfig(c *check.C) {
	const host = "123456789012.dkr.ecr.us-east-1.amazonaws.com"
	config.Set("proxy:"+host, "proxy.example.com:3128")
	defer config.Unset("proxy:" + host)

	httpClient, err := ecrHTTPClient(host)
	c.Assert(err, check.IsNil)
	c.Assert(httpClient.Timeout, check.Equals, 5*time.Minute)

	transport, ok := httpClient.Transport.(*http.Transport)
	c.Assert(ok, check.Equals, true)
	c.Assert(transport.Proxy, check.NotNil)
	req, err := http.NewRequest(http.MethodGet, "https://ecr.us-east-1.amazonaws.com/", nil)
	c.Assert(err, check.IsNil)
	proxyURL, err := transport.Proxy(req)
	c.Assert(err, check.IsNil)
	c.Assert(proxyURL.Host, check.Equals, "proxy.example.com:3128")
}

func (s *S) TestECRHTTPClientWithoutProxyConfig(c *check.C) {
	httpClient, err := ecrHTTPClient("123456789012.dkr.ecr.us-east-1.amazonaws.com")
	c.Assert(err, check.IsNil)
	c.Assert(httpClient.Timeout, check.Equals, 5*time.Minute)
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
