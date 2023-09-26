// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"fmt"
	"sort"

	"github.com/globalsign/mgo"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	tsuruEnvs "github.com/tsuru/tsuru/envs"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	"gopkg.in/mgo.v2/bson"
)

type jobService struct{}

var _ jobTypes.JobService = &jobService{}

func getProvisioner(ctx context.Context, job *jobTypes.Job) (provision.JobProvisioner, error) {

	prov, err := pool.GetProvisionerForPool(ctx, job.Pool)
	if err != nil {
		return nil, err
	}
	jobProv, ok := prov.(provision.JobProvisioner)
	if !ok {
		return nil, errors.Errorf("provisioner %q does not support native jobs and cronjobs scheduling", prov.GetName())
	}
	return jobProv, nil
}

// Units returns the list of units.
func Units(ctx context.Context, job *jobTypes.Job) ([]provision.Unit, error) {
	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return []provision.Unit{}, err
	}
	return prov.JobUnits(context.TODO(), job)
}

func JobService() (jobTypes.JobService, error) {
	return &jobService{}, nil
}

// GetByName queries the database to find a job identified by the given
// name.
func (*jobService) GetByName(ctx context.Context, name string) (*jobTypes.Job, error) {
	var job jobTypes.Job
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

func (*jobService) RemoveJobFromDb(jobName string) error {
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

func (*jobService) DeleteFromProvisioner(ctx context.Context, job *jobTypes.Job) error {
	prov, err := getProvisioner(ctx, job)
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
func (*jobService) CreateJob(ctx context.Context, job *jobTypes.Job, user *authTypes.User) error {
	jobCreationErr := jobTypes.JobCreationError{Job: job.Name}
	if err := validateName(ctx, job); err != nil {
		jobCreationErr.Err = err
		return &jobCreationErr
	}
	if err := buildPlan(ctx, job); err != nil {
		jobCreationErr.Err = err
		return &jobCreationErr
	}
	buildTsuruInfo(ctx, job, user)
	if job.Spec.Manual {
		buildFakeSchedule(ctx, job)
	}
	if err := validateJob(ctx, job); err != nil {
		return err
	}

	actions := []*action.Action{
		&reserveTeamCronjob,
		&reserveUserCronjob,
		&insertJob,
		&provisionJob,
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
func (*jobService) UpdateJob(ctx context.Context, newJob, oldJob *jobTypes.Job, user *authTypes.User) error {
	if err := newJob.Metadata.Validate(); err != nil {
		return err
	}
	oldJob.Metadata.Update(newJob.Metadata)
	newJob.Metadata = oldJob.Metadata
	// NOTE: we're merging newJob as dst in mergo, newJob is not 100% populated, it just contains the changes the user wants to make
	// in other words: we merge the non-empty values of oldJob and add to the empty values of newJob
	// TODO: add an option to erase old values, it can be easily done with mergo.Merge(dst, src, mergo.WithOverwriteWithEmptyValue),
	// in which case we would switch oldJob to be dst and newJob to be src
	if err := mergo.Merge(newJob, oldJob); err != nil {
		return err
	}
	if err := validateJob(ctx, newJob); err != nil {
		return err
	}
	actions := []*action.Action{
		&jobUpdateDB,
		&updateJobProv,
	}
	return action.NewPipeline(actions...).Execute(ctx, newJob, user)
}

func (*jobService) AddServiceEnv(ctx context.Context, job *jobTypes.Job, addArgs jobTypes.AddInstanceArgs) error {
	if len(addArgs.Envs) == 0 {
		return nil
	}

	if addArgs.Writer != nil {
		fmt.Fprintf(addArgs.Writer, "---- Setting %d new environment variables ----\n", len(addArgs.Envs)+1)
	}
	job.Spec.ServiceEnvs = append(job.Spec.ServiceEnvs, addArgs.Envs...)

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()

	err = conn.Jobs().Update(bson.M{"name": job.Name}, bson.M{"$set": bson.M{"spec.serviceenvs": job.Spec.ServiceEnvs}})
	if err != nil {
		return err
	}

	return nil
}

func (*jobService) RemoveServiceEnv(ctx context.Context, job *jobTypes.Job, removeArgs jobTypes.RemoveInstanceArgs) error {
	lenBefore := len(job.Spec.ServiceEnvs)

	for i := 0; i < len(job.Spec.ServiceEnvs); i++ {
		se := job.Spec.ServiceEnvs[i]
		if se.ServiceName == removeArgs.ServiceName && se.InstanceName == removeArgs.InstanceName {
			job.Spec.ServiceEnvs = append(job.Spec.ServiceEnvs[:i], job.Spec.ServiceEnvs[i+1:]...)
			i--
		}
	}

	toUnset := lenBefore - len(job.Spec.ServiceEnvs)
	if toUnset <= 0 {
		return nil
	}
	if removeArgs.Writer != nil {
		fmt.Fprintf(removeArgs.Writer, "---- Unsetting %d environment variables ----\n", toUnset)
	}

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()

	err = conn.Jobs().Update(bson.M{"name": job.Name}, bson.M{"$set": bson.M{"spec.serviceenvs": job.Spec.ServiceEnvs}})
	if err != nil {
		return err
	}

	return nil
}

func (*jobService) UpdateJobProv(ctx context.Context, job *jobTypes.Job) error {
	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return err
	}

	return prov.UpdateJob(ctx, job)
}

// Trigger triggers an execution of either job or cronjob object
func (*jobService) Trigger(ctx context.Context, job *jobTypes.Job) error {
	return action.NewPipeline([]*action.Action{&triggerCron}...).Execute(ctx, job)
}

func filterQuery(f *jobTypes.Filter) bson.M {
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

func (*jobService) List(ctx context.Context, filter *jobTypes.Filter) ([]jobTypes.Job, error) {
	jobs := []jobTypes.Job{}
	query := filterQuery(filter)
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

func (*jobService) GetEnvs(ctx context.Context, job *jobTypes.Job) map[string]bindTypes.EnvVar {
	mergedEnvs := make(map[string]bindTypes.EnvVar, len(job.Spec.Envs)+len(job.Spec.ServiceEnvs)+1)
	toInterpolate := make(map[string]string)
	var toInterpolateKeys []string

	for _, e := range job.Spec.Envs {
		mergedEnvs[e.Name] = e
		if e.Alias != "" {
			toInterpolate[e.Name] = e.Alias
			toInterpolateKeys = append(toInterpolateKeys, e.Name)
		}
	}

	for _, e := range job.Spec.ServiceEnvs {
		mergedEnvs[e.Name] = e.EnvVar
	}
	sort.Strings(toInterpolateKeys)

	for _, envName := range toInterpolateKeys {
		tsuruEnvs.Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}

	mergedEnvs[tsuruEnvs.TsuruServicesEnvVar] = tsuruEnvs.ServiceEnvsFromEnvVars(job.Spec.ServiceEnvs)

	return mergedEnvs
}

func SetEnvs(ctx context.Context, job *jobTypes.Job, setEnvs bind.SetEnvArgs) error {
	if setEnvs.ManagedBy == "" && len(setEnvs.Envs) == 0 {
		return nil
	}

	if setEnvs.Writer != nil && len(setEnvs.Envs) > 0 {
		fmt.Fprintf(setEnvs.Writer, "---- Setting %d new environment variables ----\n", len(setEnvs.Envs))
	}

	mapEnvs := map[string]bindTypes.EnvVar{}
	for _, env := range job.Spec.Envs {
		mapEnvs[env.Name] = env
	}

	if setEnvs.PruneUnused {
		for _, env := range job.Spec.Envs {
			index := indexEnvInSet(env.Name, setEnvs.Envs)
			// only prune variables managed by requested
			if index == -1 && env.ManagedBy == setEnvs.ManagedBy {
				if setEnvs.Writer != nil {
					fmt.Fprintf(setEnvs.Writer, "---- Pruning %s from environment variables ----\n", env.Name)
					delete(mapEnvs, env.Name)
				}
			}
		}
	}

	for _, env := range setEnvs.Envs {
		mapEnvs[env.Name] = env
	}

	job.Spec.Envs = []bindTypes.EnvVar{}
	for _, env := range mapEnvs {
		job.Spec.Envs = append(job.Spec.Envs, env)
	}
	sort.Slice(job.Spec.Envs, func(i, j int) bool {
		return job.Spec.Envs[i].Name < job.Spec.Envs[j].Name
	})

	err := updateJobDB(ctx, job)
	if err != nil {
		return err
	}

	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return err
	}
	return prov.UpdateJob(ctx, job)

}

func UnsetEnvs(ctx context.Context, job *jobTypes.Job, unsetEnvs bind.UnsetEnvArgs) error {
	if len(unsetEnvs.VariableNames) == 0 {
		return nil
	}
	if unsetEnvs.Writer != nil {
		fmt.Fprintf(unsetEnvs.Writer, "---- Unsetting %d environment variables ----\n", len(unsetEnvs.VariableNames))
	}

	mapEnvs := map[string]bindTypes.EnvVar{}
	for _, env := range job.Spec.Envs {
		mapEnvs[env.Name] = env
	}
	for _, name := range unsetEnvs.VariableNames {
		delete(mapEnvs, name)
	}
	job.Spec.Envs = []bindTypes.EnvVar{}
	for _, env := range mapEnvs {
		job.Spec.Envs = append(job.Spec.Envs, env)
	}
	sort.Slice(job.Spec.Envs, func(i, j int) bool {
		return job.Spec.Envs[i].Name < job.Spec.Envs[j].Name
	})

	err := updateJobDB(ctx, job)
	if err != nil {
		return err
	}

	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return err
	}
	return prov.UpdateJob(ctx, job)
}

func indexEnvInSet(envName string, envs []bindTypes.EnvVar) int {
	for i, e := range envs {
		if e.Name == envName {
			return i
		}
	}
	return -1
}

func validatePool(ctx context.Context, job *jobTypes.Job) error {
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

func validateTeamOwner(ctx context.Context, job *jobTypes.Job, p *pool.Pool) error {
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

func validateJob(ctx context.Context, j *jobTypes.Job) error {
	if err := validatePool(ctx, j); err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	if err := validatePlan(ctx, j.Pool, j.Plan.Name); err != nil {
		return err
	}
	if !j.Spec.Manual {
		c := cron.New()
		if _, err := c.AddFunc(j.Spec.Schedule, func() {}); err != nil {
			return &tsuruErrors.ValidationError{Message: jobTypes.ErrInvalidSchedule.Error()}
		}
	}
	return nil
}
