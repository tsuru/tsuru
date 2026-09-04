// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"regexp"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/pkg/errors"
)

// Matches standard, FIPS and China partition ECR registry hosts, capturing the region.
var ecrRegistryRegexp = regexp.MustCompile(`^[0-9]{12}\.dkr\.ecr(-fips)?\.([a-z0-9-]+)\.amazonaws\.com(\.cn)?$`)

func isECRRegistry(host string) (region string, ok bool) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	m := ecrRegistryRegexp.FindStringSubmatch(host)
	if m == nil {
		return "", false
	}
	return m[2], true
}

type ecrClient interface {
	DescribeImages(ctx context.Context, params *ecr.DescribeImagesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error)
	BatchDeleteImage(ctx context.Context, params *ecr.BatchDeleteImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error)
}

var (
	ecrClientMu  sync.Mutex
	ecrClients   = map[string]ecrClient{}
	newECRClient = defaultECRClient
)

func defaultECRClient(ctx context.Context, region string) (ecrClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load AWS config for region %s", region)
	}
	return ecr.NewFromConfig(cfg), nil
}

func ecrClientForRegion(ctx context.Context, region string) (ecrClient, error) {
	ecrClientMu.Lock()
	client, ok := ecrClients[region]
	ecrClientMu.Unlock()
	if ok {
		return client, nil
	}
	client, err := newECRClient(ctx, region)
	if err != nil {
		return nil, err
	}
	ecrClientMu.Lock()
	defer ecrClientMu.Unlock()
	if cached, ok := ecrClients[region]; ok {
		return cached, nil
	}
	ecrClients[region] = client
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
func removeECRImage(ctx context.Context, region, repository, tag string) error {
	client, err := ecrClientForRegion(ctx, region)
	if err != nil {
		return err
	}

	digest, err := ecrImageDigest(ctx, client, repository, tag)
	if err != nil {
		return err
	}

	out, err := client.BatchDeleteImage(ctx, &ecr.BatchDeleteImageInput{
		RepositoryName: aws.String(repository),
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageDigest: aws.String(digest)}},
	})
	if err != nil {
		if isECRNotFound(err) {
			return ErrImageNotFound
		}
		return errors.Wrapf(err, "failed to remove image %s:%s on ECR", repository, tag)
	}

	// BatchDeleteImage reports per-image problems in the response body rather
	// than as an error. A single image was requested, so at most one failure.
	if len(out.Failures) > 0 {
		f := out.Failures[0]
		if f.FailureCode == ecrtypes.ImageFailureCodeImageNotFound {
			return ErrImageNotFound
		}
		return errors.Errorf("failed to remove image %s:%s on ECR: %s: %s", repository, tag, f.FailureCode, aws.ToString(f.FailureReason))
	}
	return nil
}

// ecrImageDigest resolves a tag to the digest of the manifest it points at.
// DescribeImages is used instead of BatchGetImage because it reports the digest
// for any manifest media type, including image indexes.
func ecrImageDigest(ctx context.Context, client ecrClient, repository, tag string) (string, error) {
	out, err := client.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repository),
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageTag: aws.String(tag)}},
	})
	if err != nil {
		if isECRNotFound(err) {
			return "", ErrImageNotFound
		}
		return "", errors.Wrapf(err, "failed to get digest for image %s:%s on ECR", repository, tag)
	}
	if len(out.ImageDetails) == 0 || out.ImageDetails[0].ImageDigest == nil {
		return "", ErrImageNotFound
	}
	return aws.ToString(out.ImageDetails[0].ImageDigest), nil
}

func isECRNotFound(err error) bool {
	var repoNotFound *ecrtypes.RepositoryNotFoundException
	var imageNotFound *ecrtypes.ImageNotFoundException
	return errors.As(err, &repoNotFound) || errors.As(err, &imageNotFound)
}
