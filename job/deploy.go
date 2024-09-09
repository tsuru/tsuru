// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"io"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/provision"

	jobTypes "github.com/tsuru/tsuru/types/job"
)

func (*jobService) Deploy(ctx context.Context, opts jobTypes.DeployOptions, job *jobTypes.Job, output io.Writer) (string, error) {
	jobProv, err := getProvisioner(ctx, job)
	if err != nil {
		return "", err
	}

	prov := jobProv.(provision.Provisioner)
	jobBuilder, err := builder.GetForProvisioner(prov)
	if err != nil {
		return "", err
	}

	imageDst, err := jobBuilder.BuildJob(ctx, job, builder.BuildOpts{
		ImageID:     opts.Image,
		ArchiveFile: opts.File,
		ArchiveSize: opts.FileSize,
		Dockerfile:  opts.Dockerfile,
		Message:     opts.Message,
		Output:      output,
	})
	if err != nil {
		return "", err
	}

	job.Spec.Container.InternalRegistryImage = imageDst

	jobProv, ok := prov.(provision.JobProvisioner)
	if !ok {
		return "", errors.Errorf("provisioner %q does not support native jobs and cronjobs scheduling", prov.GetName())
	}

	err = updateJobDB(ctx, job)
	if err != nil {
		return "", err
	}

	return imageDst, jobProv.EnsureJob(ctx, job)
}
