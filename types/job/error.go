// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"errors"
	"fmt"
)

var (
	ErrJobNotFound        = errors.New("Job not found")
	ErrJobUnitNotFound    = errors.New("Job unit not found")
	MaxAttempts           = 5
	ErrMaxAttemptsReached = fmt.Errorf("Unable to generate unique job name: max attempts reached (%d)", MaxAttempts)
	ErrJobAlreadyExists   = errors.New("a job with the same name already exists")
	ErrInvalidSchedule    = errors.New("invalid schedule")
)

type JobCreationError struct {
	Job string
	Err error
}

func (e *JobCreationError) Error() string {
	return fmt.Sprintf("tsuru failed to create job %q: %s", e.Job, e.Err)
}
