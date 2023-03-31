// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"fmt"

	"github.com/adhocore/gronx"
	"github.com/globalsign/mgo"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	"gopkg.in/mgo.v2/bson"
)

// JobSpec defines how a job should be run, i.e if it has a schedule it will be created as a CronJob object
type JobSpec struct {
	// Specifies the maximum desired number of pods the job should
	// run at any given time. The actual number of pods running in steady state will
	// be less than this number when ((.spec.completions - .status.successful) < .spec.parallelism),
	// i.e. when the work left to do is less than max parallelism.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	// +optional
	Parallelism *int32

	// Specifies the desired number of successfully finished pods the
	// job should be run with.  Setting to nil means that the success of any
	// pod signals the success of all pods, and allows parallelism to have any positive
	// value.  Setting to 1 means that parallelism is limited to 1 and the success of that
	// pod signals the success of the job.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	// +optional
	Completions *int32

	// Specifies the duration in seconds relative to the startTime that the job may be active
	// before the system tries to terminate it; value must be positive integer
	// +optional
	ActiveDeadlineSeconds *int64

	// Specifies the number of retries before marking this job failed.
	// Defaults to 6
	// +optional
	BackoffLimit *int32

	Schedule string

	Container jobTypes.ContainerInfo
}

// Job is another main type in tsuru as of version 1.13
// a job currently represents a Kubernetes Job object or a Cronjob object
// this struct carries some tsuru metadata as is the case with the app object
// it also holds a JobSpec value that defines how the Job is supposed to be run
type Job struct {
	Name        string
	Teams       []string
	TeamOwner   string
	Owner       string
	Plan        appTypes.Plan
	Metadata    appTypes.Metadata
	Pool        string
	Description string

	Spec JobSpec

	provisioner provision.JobProvisioner
}

func (job *Job) getProvisioner(ctx context.Context) (provision.JobProvisioner, error) {
	var err error
	if job.provisioner != nil {
		return job.provisioner, nil
	}
	prov, err := pool.GetProvisionerForPool(ctx, job.Pool)
	if err != nil {
		return nil, err
	}
	jobProv, ok := prov.(provision.JobProvisioner)
	if !ok {
		return nil, errors.Errorf("provisioner %q does not support native jobs and cronjobs scheduling", prov.GetName())
	}
	job.provisioner = jobProv
	return job.provisioner, nil
}

// Units returns the list of units.
func (job *Job) Units(ctx context.Context) ([]provision.Unit, error) {
	prov, err := job.getProvisioner(ctx)
	if err != nil {
		return []provision.Unit{}, err
	}
	return prov.JobUnits(context.TODO(), job)
}

func (job *Job) GetName() string {
	return job.Name
}

// GetMemory returns the memory limit (in bytes) for the job.
func (job *Job) GetMemory() int64 {
	if job.Plan.Override.Memory != nil {
		return *job.Plan.Override.Memory
	}
	return job.Plan.Memory
}

func (job *Job) GetMilliCPU() int {
	if job.Plan.Override.CPUMilli != nil {
		return *job.Plan.Override.CPUMilli
	}
	return job.Plan.CPUMilli
}

func (job *Job) GetPool() string {
	return job.Pool
}

func (job *Job) GetTeamOwner() string {
	return job.TeamOwner
}
func (job *Job) GetTeamsName() []string {
	return job.Teams
}

func (job *Job) GetMetadata() appTypes.Metadata {
	return job.Metadata
}

func (job *Job) IsCron() bool {
	return job.Spec.Schedule != ""
}

func (job *Job) GetSchedule() string {
	return job.Spec.Schedule
}

func (job *Job) GetSpec() jobTypes.JobSpec {
	return jobTypes.JobSpec{
		Parallelism:           job.Spec.Parallelism,
		Completions:           job.Spec.Completions,
		ActiveDeadlineSeconds: job.Spec.ActiveDeadlineSeconds,
		BackoffLimit:          job.Spec.BackoffLimit,
		Schedule:              job.Spec.Schedule,
		ContainerInfo:         job.Spec.Container,
	}
}

// GetByName queries the database to find a job identified by the given
// name.
func GetByName(ctx context.Context, name string) (*Job, error) {
	var job Job
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Jobs().Find(bson.M{"name": name}).One(&job)
	if err == mgo.ErrNotFound {
		return nil, jobTypes.ErrJobNotFound
	}
	return &job, err
}

func RemoveJobFromDb(jobName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Jobs().Remove(bson.M{"name": jobName})
	if err == mgo.ErrNotFound {
		return jobTypes.ErrJobNotFound
	}
	return err
}

func DeleteFromProvisioner(ctx context.Context, job *Job) error {
	prov, err := job.getProvisioner(ctx)
	if err != nil {
		return err
	}
	return prov.DestroyJob(ctx, job)
}

// CreateJob creates a new job or cronjob.
//
// Creating a new job is a process composed of the following steps:
//
//  1. Save the job in the database
//  2. Provision the job using the provisioner
func CreateJob(ctx context.Context, job *Job, user *auth.User, trigger bool) error {
	jobCreationErr := jobTypes.JobCreationError{Job: job.Name}
	if err := buildName(ctx, job); err != nil {
		jobCreationErr.Err = err
		return &jobCreationErr
	}
	if err := buildPlan(ctx, job); err != nil {
		jobCreationErr.Err = err
		return &jobCreationErr
	}
	buildTsuruInfo(ctx, job, user)

	if err := validateJob(ctx, *job); err != nil {
		return err
	}

	var actions []*action.Action
	if job.IsCron() {
		if trigger {
			jobCreationErr.Err = errors.New("can't create and forcefully run a cronjob at the same time, please create the cronjob first then trigger a manual run or just create a job with --run")
			return &jobCreationErr
		}
		actions = []*action.Action{
			&reserveTeamCronjob,
			&reserveUserCronjob,
			&insertJob,
			&provisionJob,
		}
	} else {
		actions = []*action.Action{
			&insertJob,
		}
		if trigger {
			actions = append(actions, &provisionJob)
		}
	}

	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(ctx, job, user)
}

// UpdateJob updates an existing cronjob.
//
// Updating a new job is a process composed of the following steps:
//
//  1. Patch the job using the provisioner
//  2. Update the job in the database
func UpdateJob(ctx context.Context, newJob, oldJob *Job, user *auth.User) error {
	if err := mergo.Merge(newJob, oldJob); err != nil {
		return err
	}
	if err := validateJob(ctx, *newJob); err != nil {
		return err
	}
	actions := []*action.Action{
		&jobUpdateDB,
	}
	if newJob.IsCron() {
		actions = append(actions, &updateJobProv)
	}
	return action.NewPipeline(actions...).Execute(ctx, newJob, user)
}

// Trigger triggers an execution of either job or cronjob object
func Trigger(ctx context.Context, job *Job) error {
	var actions []*action.Action
	if job.IsCron() {
		actions = []*action.Action{&triggerCron}
	} else {
		actions = []*action.Action{&provisionJob}
	}
	return action.NewPipeline(actions...).Execute(ctx, job)
}

type Filter struct {
	Name      string
	TeamOwner string
	UserOwner string
	Pool      string
	Pools     []string
	Extra     map[string][]string
}

func (f *Filter) ExtraIn(name string, value string) {
	if f.Extra == nil {
		f.Extra = make(map[string][]string)
	}
	f.Extra[name] = append(f.Extra[name], value)
}

func (f *Filter) Query() bson.M {
	if f == nil {
		return bson.M{}
	}
	query := bson.M{}
	if f.Extra != nil {
		var orBlock []bson.M
		for field, values := range f.Extra {
			orBlock = append(orBlock, bson.M{
				field: bson.M{"$in": values},
			})
		}
		query["$or"] = orBlock
	}
	if f.Name != "" {
		query["name"] = bson.M{"$regex": f.Name}
	}
	if f.TeamOwner != "" {
		query["teamowner"] = f.TeamOwner
	}
	if f.UserOwner != "" {
		query["owner"] = f.UserOwner
	}
	if f.Pool != "" {
		query["pool"] = f.Pool
	}
	if len(f.Pools) > 0 {
		query["pool"] = bson.M{"$in": f.Pools}
	}
	return query
}

func List(ctx context.Context, filter *Filter) ([]Job, error) {
	jobs := []Job{}
	query := filter.Query()
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	err = conn.Jobs().Find(query).All(&jobs)
	conn.Close()
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// LastLogs returns a list of the last `lines` log of the app, matching the
// fields in the log instance received as an example.
func (job *Job) LastLogs(ctx context.Context, logService appTypes.AppLogService, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	args.Name = job.Name
	args.Type = "job"
	return logService.List(ctx, args)
}

func validateSchedule(jobName, schedule string) error {
	gronx := gronx.New()
	if !gronx.IsValid(schedule) {
		return jobTypes.ErrInvalidSchedule
	}
	return nil
}

func validatePool(ctx context.Context, job Job) error {
	p, err := pool.GetPoolByName(ctx, job.Pool)
	if err != nil {
		return err
	}
	return validateTeamOwner(ctx, job, p)
}

func validatePlan(ctx context.Context, poolName, planName string) error {
	pool, err := pool.GetPoolByName(ctx, poolName)
	if err != nil {
		return err
	}
	plans, err := pool.GetPlans()
	if err != nil {
		return err
	}
	planSet := set.FromSlice(plans)
	if !planSet.Includes(planName) {
		msg := fmt.Sprintf("Job plan %q is not allowed on pool %q", planName, pool.Name)
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return nil
}

func validateTeamOwner(ctx context.Context, job Job, p *pool.Pool) error {
	_, err := servicemanager.Team.FindByName(ctx, job.TeamOwner)
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	poolTeams, err := p.GetTeams()
	if err != nil && err != pool.ErrPoolHasNoTeam {
		msg := fmt.Sprintf("failed to get pool %q teams", p.Name)
		return &tsuruErrors.ValidationError{Message: msg}
	}
	for _, team := range poolTeams {
		if team == job.TeamOwner {
			return nil
		}
	}
	msg := fmt.Sprintf("Job team owner %q has no access to pool %q", job.TeamOwner, p.Name)
	return &tsuruErrors.ValidationError{Message: msg}
}

func validateJob(ctx context.Context, j Job) error {
	if err := validatePool(ctx, j); err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	if err := validatePlan(ctx, j.Pool, j.Plan.Name); err != nil {
		return err
	}
	if j.IsCron() {
		if err := validateSchedule(j.Name, j.Spec.Schedule); err != nil {
			return &tsuruErrors.ValidationError{Message: err.Error()}
		}
	}
	return nil
}
