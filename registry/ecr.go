// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/pkg/errors"
	tsuruNet "github.com/tsuru/tsuru/net"
)

// Matches standard, FIPS and China partition ECR registry hosts, capturing
// the account ID, the FIPS marker and the region.
var ecrRegistryRegexp = regexp.MustCompile(`^([0-9]{12})\.dkr\.ecr(-fips)?\.([a-z0-9-]+)\.amazonaws\.com(\.cn)?$`)

// isECRRegistry reports whether host is an ECR registry endpoint, returning
// the account ID embedded in the host, the region and whether it is a FIPS
// endpoint. The account ID is the registry the host actually points at and
// must be passed as RegistryId on every ECR API call: omitting it makes the
// SDK default to the caller identity's own account, which is wrong whenever
// tsuru's credentials are used cross-account.
func isECRRegistry(host string) (accountID, region string, fips, ok bool) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	m := ecrRegistryRegexp.FindStringSubmatch(host)
	if m == nil {
		return "", "", false, false
	}
	return m[1], m[3], m[2] != "", true
}

type ecrClient interface {
	DescribeImages(ctx context.Context, params *ecr.DescribeImagesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error)
	BatchDeleteImage(ctx context.Context, params *ecr.BatchDeleteImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error)
}

var (
	ecrClientMu sync.Mutex
	// ecrClients is keyed by the registry host verbatim: the host alone
	// determines account, region, FIPS variant and the proxy:<host> config
	// tsuru's Distribution path already honors, so caching by host keeps a
	// distinct client per distinct proxy configuration for free.
	ecrClients   = map[string]ecrClient{}
	newECRClient = defaultECRClient
)

// ecrHTTPClient builds the HTTP client used for ECR API calls: the same
// dial/full-request timeouts tsuru's Distribution registry path uses
// (Dial15Full300ClientNoKeepAlive), routed through the proxy:<host> config
// entry for host, if any, exactly as that path does for host.
func ecrHTTPClient(host string) (*http.Client, error) {
	return tsuruNet.WithProxyFromConfig(*tsuruNet.Dial15Full300ClientNoKeepAlive, host)
}

// defaultECRClient builds an ECR client whose HTTP transport matches the
// Distribution path's: same dial/full-request timeouts, and routed through
// the proxy:<host> config entry for host, if any, so installations that rely
// on that setting for network egress keep working for ECR GC and app removal.
func defaultECRClient(ctx context.Context, host, region string, fips bool) (ecrClient, error) {
	httpClient, err := ecrHTTPClient(host)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to configure HTTP client for ECR host %s", host)
	}
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
		awsconfig.WithHTTPClient(httpClient),
	}
	if fips {
		opts = append(opts, awsconfig.WithUseFIPSEndpoint(aws.FIPSEndpointStateEnabled))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load AWS config for region %s", region)
	}
	return ecr.NewFromConfig(cfg), nil
}

func ecrClientForHost(ctx context.Context, host, region string, fips bool) (ecrClient, error) {
	ecrClientMu.Lock()
	client, ok := ecrClients[host]
	ecrClientMu.Unlock()
	if ok {
		return client, nil
	}
	client, err := newECRClient(ctx, host, region, fips)
	if err != nil {
		return nil, err
	}
	ecrClientMu.Lock()
	defer ecrClientMu.Unlock()
	if cached, ok := ecrClients[host]; ok {
		return cached, nil
	}
	ecrClients[host] = client
	return client, nil
}

// removeECRImage deletes an image from an ECR repository. ECR does not
// implement the Docker Registry v2 manifest DELETE endpoint, so deletions must
// go through the ECR API instead.
//
// Deletion is done by digest: BatchDeleteImage on a tag only removes that tag,
// keeping the manifest around while any other tag points at it, and tsuru
// pushes the same image under both its version tag and "latest". Deleting the
// manifest removes every tag on it, matching the Distribution path.
func removeECRImage(ctx context.Context, host, accountID, region string, fips bool, repository, tag string) error {
	client, err := ecrClientForHost(ctx, host, region, fips)
	if err != nil {
		return err
	}

	digest, err := ecrImageDigest(ctx, client, accountID, repository, tag)
	if err != nil {
		return err
	}

	out, err := client.BatchDeleteImage(ctx, &ecr.BatchDeleteImageInput{
		RegistryId:     aws.String(accountID),
		RepositoryName: aws.String(repository),
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageDigest: aws.String(digest)}},
	})
	if err != nil {
		if isECRNotFound(err) {
			return errors.Wrapf(ErrImageNotFound, "image %s:%s on ECR", repository, tag)
		}
		return errors.Wrapf(err, "failed to remove image %s:%s on ECR", repository, tag)
	}

	// BatchDeleteImage reports per-image problems in the response body rather
	// than as an error. A single image was requested, so at most one failure.
	if len(out.Failures) > 0 {
		f := out.Failures[0]
		if f.FailureCode == ecrtypes.ImageFailureCodeImageNotFound {
			return errors.Wrapf(ErrImageNotFound, "image %s:%s on ECR", repository, tag)
		}
		return errors.Errorf("failed to remove image %s:%s on ECR: %s: %s", repository, tag, f.FailureCode, aws.ToString(f.FailureReason))
	}
	return nil
}

// ecrImageDigest resolves a tag to the digest of the manifest it points at.
// DescribeImages is used instead of BatchGetImage because it reports the digest
// for any manifest media type, including image indexes.
//
// A single ImageIdentifier filtered by ImageTag returns at most one
// ImageDetail: an ECR tag points at exactly one digest at a time, so
// out.ImageDetails[0] is the only possible match.
func ecrImageDigest(ctx context.Context, client ecrClient, accountID, repository, tag string) (string, error) {
	out, err := client.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RegistryId:     aws.String(accountID),
		RepositoryName: aws.String(repository),
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageTag: aws.String(tag)}},
	})
	if err != nil {
		if isECRNotFound(err) {
			return "", errors.Wrapf(ErrImageNotFound, "image %s:%s on ECR", repository, tag)
		}
		return "", errors.Wrapf(err, "failed to get digest for image %s:%s on ECR", repository, tag)
	}
	if len(out.ImageDetails) == 0 || out.ImageDetails[0].ImageDigest == nil {
		return "", errors.Wrapf(ErrImageNotFound, "image %s:%s on ECR", repository, tag)
	}
	return aws.ToString(out.ImageDetails[0].ImageDigest), nil
}

func isECRNotFound(err error) bool {
	var repoNotFound *ecrtypes.RepositoryNotFoundException
	var imageNotFound *ecrtypes.ImageNotFoundException
	return errors.As(err, &repoNotFound) || errors.As(err, &imageNotFound)
}
