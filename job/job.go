// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"fmt"
	"sort"

	"github.com/adhocore/gronx"
	"github.com/globalsign/mgo"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	"gopkg.in/mgo.v2/bson"
)

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

// GetByName queries the database to find a job identified by the given
// name.
func GetByName(ctx context.Context, name string) (*jobTypes.Job, error) {
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

func DeleteFromProvisioner(ctx context.Context, job *jobTypes.Job) error {
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
func CreateJob(ctx context.Context, job *jobTypes.Job, user *auth.User, trigger bool) error {
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

	if err := validateJob(ctx, job); err != nil {
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
func UpdateJob(ctx context.Context, newJob, oldJob *jobTypes.Job, user *auth.User) error {
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
	}
	if newJob.IsCron() {
		actions = append(actions, &updateJobProv)
	}
	return action.NewPipeline(actions...).Execute(ctx, newJob, user)
}

// Trigger triggers an execution of either job or cronjob object
func Trigger(ctx context.Context, job *jobTypes.Job) error {
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

func List(ctx context.Context, filter *Filter) ([]jobTypes.Job, error) {
	jobs := []jobTypes.Job{}
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

func validateSchedule(jobName, schedule string) error {
	gronx := gronx.New()
	if !gronx.IsValid(schedule) {
		return jobTypes.ErrInvalidSchedule
	}
	return nil
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
	if j.IsCron() {
		if err := validateSchedule(j.Name, j.Spec.Schedule); err != nil {
			return &tsuruErrors.ValidationError{Message: err.Error()}
		}
	}
	return nil
}
