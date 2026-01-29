// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruEnvs "github.com/tsuru/tsuru/envs"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	"github.com/tsuru/tsuru/streamfmt"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	provTypes "github.com/tsuru/tsuru/types/provision"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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
func Units(ctx context.Context, job *jobTypes.Job) ([]provTypes.Unit, error) {
	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return []provTypes.Unit{}, err
	}
	return prov.JobUnits(context.TODO(), job)
}

func JobService() (jobTypes.JobService, error) {
	return &jobService{}, nil
}

func (*jobService) KillUnit(ctx context.Context, job *jobTypes.Job, unit string, force bool) error {
	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return err
	}
	return prov.KillJobUnit(ctx, job, unit, force)
}

// GetByName queries the database to find a job identified by the given
// name.
func (*jobService) GetByName(ctx context.Context, name string) (*jobTypes.Job, error) {
	var job jobTypes.Job
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return nil, err
	}
	err = collection.FindOne(ctx, mongoBSON.M{"name": name}).Decode(&job)
	if err == mongo.ErrNoDocuments {
		return nil, jobTypes.ErrJobNotFound
	}
	return &job, err
}

func (*jobService) RemoveJob(ctx context.Context, job *jobTypes.Job) error {
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return err
	}
	result, err := collection.DeleteOne(ctx, mongoBSON.M{"name": job.Name})
	if err == mongo.ErrNoDocuments {
		return jobTypes.ErrJobNotFound
	}
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return jobTypes.ErrJobNotFound
	}

	servicemanager.TeamQuota.Inc(ctx, &authTypes.Team{Name: job.TeamOwner}, -1)
	var user *auth.User
	if user, err = auth.GetUserByEmail(ctx, job.Owner); err == nil {
		servicemanager.UserQuota.Inc(ctx, user, -1)
	}
	return nil
}

func (*jobService) RemoveJobProv(ctx context.Context, job *jobTypes.Job) error {
	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return err
	}
	return prov.DestroyJob(ctx, job)
}

// builderDeploy uses deploy-agent to push the image to tsuru's registry and deploy the job using the new pushed image
func builderDeploy(ctx context.Context, job *jobTypes.Job, opts builder.BuildOpts) (string, error) {
	if err := ctx.Err(); err != nil { // e.g. context deadline exceeded
		return "", err
	}

	if job == nil {
		return "", errors.New("job not provided")
	}

	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return "", err
	}
	builder, err := builder.GetForProvisioner(prov.(provision.Provisioner))
	if err != nil {
		return "", err
	}
	return builder.BuildJob(ctx, job, opts)
}

func getRegistry(ctx context.Context, job *jobTypes.Job) (imgTypes.ImageRegistry, error) {
	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return "", err
	}
	registryProv, ok := prov.(provision.MultiRegistryProvisioner)
	if !ok {
		return "", nil
	}
	return registryProv.RegistryForPool(ctx, job.Pool)
}

func (*jobService) BaseImageName(ctx context.Context, job *jobTypes.Job) (string, error) {
	reg, err := getRegistry(ctx, job)
	if err != nil {
		return "", err
	}
	newImage, err := image.JobBasicImageName(reg, job.Name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:latest", newImage), nil
}

func imageID(job *jobTypes.Job) (string, error) {
	var imageID string
	if job.DeployOptions != nil && job.DeployOptions.Image != "" {
		imageID = job.DeployOptions.Image
	}
	if imageID == "" {
		imageID = job.Spec.Container.OriginalImageSrc
	}
	if imageID == "" {
		return "", errors.New("no image provided")
	}
	return imageID, nil
}

func buildWithDeployAgent(ctx context.Context, job *jobTypes.Job) error {
	imageID, err := imageID(job)
	if err != nil {
		return err
	}
	newImageDst, err := builderDeploy(ctx, job, builder.BuildOpts{
		ImageID: imageID,
	})
	defer func() {
		job.Spec.Container.InternalRegistryImage = newImageDst
	}()
	if err != nil {
		return err
	}
	return nil
}

func ensureDeployOptions(j *jobTypes.Job) error {
	if j.DeployOptions != nil {
		// TODO: remove this when we remove the old deploy kind from the client side
		// this makes sure OriginalImageSrc is always populated
		j.Spec.Container.OriginalImageSrc = j.DeployOptions.Image
		return nil
	}
	// TODO: remove this when we remove the old deploy kind from the client side
	if j.Spec.Container.OriginalImageSrc != "" {
		j.DeployOptions = &jobTypes.DeployOptions{
			Kind:  provTypes.DeployImage,
			Image: j.Spec.Container.OriginalImageSrc,
		}
		return nil
	}

	j.DeployOptions = &jobTypes.DeployOptions{}

	return nil //nil to allow creation without image
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
	buildTsuruInfo(job, user)
	if job.Spec.Manual {
		buildFakeSchedule(job)
	}
	if job.Spec.ActiveDeadlineSeconds == nil {
		job.Spec.ActiveDeadlineSeconds = func(i int64) *int64 { return &i }(0)
	}
	if err := validateJob(ctx, job); err != nil {
		return err
	}

	if err := ensureDeployOptions(job); err != nil {
		return err
	}

	actions := []*action.Action{
		&reserveTeamCronjob,
		&reserveUserCronjob,
		&insertJob,
	}

	if job.DeployOptions.Image != "" {
		err := buildWithDeployAgent(ctx, job)
		if err != nil {
			return &jobTypes.JobCreationError{Job: job.Name, Err: err}
		}

		actions = append(actions, &provisionJob)
	}

	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(ctx, job, user)
}

// updateDeployOptions updates the job's deployOptions if the job's image has changed
func updateDeployOptions(oldJob, newJob *jobTypes.Job) (hasChanged bool, err error) {
	// we have to ensure oldJob has deployOptions, for compatibility with legacy way of creating jobs
	if err := ensureDeployOptions(oldJob); err != nil {
		return false, err
	}
	// if newJob doesn't have any info about deployOptions, it means client did not pass job.Spec.Container.Image nor job.DeployOptions
	// so it doesnt want to change the deployOptions
	// we use the oldJob's deployOptions forcing reflect to pass the validation and skip buildWithDeployAgent
	if err := ensureDeployOptions(newJob); err != nil {
		if err == jobTypes.ErrInvalidDeployKind {
			newJob.DeployOptions = oldJob.DeployOptions
		} else {
			return false, err
		}
	}
	return !reflect.DeepEqual(newJob.DeployOptions, oldJob.DeployOptions), nil
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
	manualJob := oldJob.Spec.Manual
	if newJob.Spec.Schedule != "" {
		manualJob = false
	}
	if newJob.Spec.Manual {
		manualJob = true
		buildFakeSchedule(newJob)
	}
	newJobActiveDeadlineSeconds := buildActiveDeadline(newJob.Spec.ActiveDeadlineSeconds)

	deployOptionsHasChanged, updateErr := updateDeployOptions(oldJob, newJob)
	if updateErr != nil {
		return updateErr
	}

	// NOTE: we're merging newJob as dst in mergo, newJob is not 100% populated, it just contains the changes the user wants to make
	// in other words: we merge the non-empty values of oldJob and add to the empty values of newJob
	// TODO: add an option to erase old values, it can be easily done with mergo.Merge(dst, src, mergo.WithOverwriteWithEmptyValue),
	// in which case we would switch oldJob to be dst and newJob to be src
	if err := mergo.Merge(newJob, oldJob); err != nil {
		return err
	}

	if deployOptionsHasChanged {
		err := buildWithDeployAgent(ctx, newJob)
		if err != nil {
			return err
		}
	}

	if newJobActiveDeadlineSeconds != nil {
		newJob.Spec.ActiveDeadlineSeconds = newJobActiveDeadlineSeconds
	}
	newJob.Spec.Manual = manualJob
	if err := buildPlan(ctx, newJob); err != nil {
		return err
	}
	if err := validateJob(ctx, newJob); err != nil {
		return err
	}

	actions := []*action.Action{
		&jobUpdateDB,
	}

	if shouldUpdateJobProvision(newJob) {
		actions = append(actions, &updateJobProv)
	}

	return action.NewPipeline(actions...).Execute(ctx, newJob, user)
}

func (*jobService) AddServiceEnv(ctx context.Context, job *jobTypes.Job, addArgs jobTypes.AddInstanceArgs) error {
	if len(addArgs.Envs) == 0 {
		return nil
	}

	if addArgs.Writer != nil {
		streamfmt.FprintlnSectionf(addArgs.Writer, "Setting %d new environment variables", len(addArgs.Envs)+1)
	}
	job.Spec.ServiceEnvs = append(job.Spec.ServiceEnvs, addArgs.Envs...)

	collection, err := storagev2.JobsCollection()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": job.Name}, mongoBSON.M{"$set": mongoBSON.M{"spec.serviceenvs": job.Spec.ServiceEnvs}})
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
		streamfmt.FprintlnSectionf(removeArgs.Writer, "Unsetting %d environment variables", toUnset)
	}

	collection, err := storagev2.JobsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": job.Name}, mongoBSON.M{"$set": mongoBSON.M{"spec.serviceenvs": job.Spec.ServiceEnvs}})
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

	return prov.EnsureJob(ctx, job)
}

// Trigger triggers an execution of either job or cronjob object
func (*jobService) Trigger(ctx context.Context, job *jobTypes.Job) error {
	return action.NewPipeline([]*action.Action{&triggerCron}...).Execute(ctx, job)
}

func processTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	processedTags := []string{}
	usedTags := make(map[string]bool)
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if len(tag) > 0 && !usedTags[tag] {
			processedTags = append(processedTags, tag)
			usedTags[tag] = true
		}
	}
	return processedTags
}

func filterQuery(f *jobTypes.Filter) mongoBSON.M {
	if f == nil {
		return mongoBSON.M{}
	}
	query := mongoBSON.M{}
	if f.Extra != nil {
		var orBlock []mongoBSON.M
		for field, values := range f.Extra {
			orBlock = append(orBlock, mongoBSON.M{
				field: mongoBSON.M{"$in": values},
			})
		}
		query["$or"] = orBlock
	}
	if f.Name != "" {
		query["name"] = mongoBSON.M{"$regex": f.Name}
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
		query["pool"] = mongoBSON.M{"$in": f.Pools}
	}
	tags := processTags(f.Tags)
	if len(tags) > 0 {
		query["tags"] = mongoBSON.M{"$all": tags}
	}
	return query
}

func (*jobService) List(ctx context.Context, filter *jobTypes.Filter) ([]jobTypes.Job, error) {
	jobs := []jobTypes.Job{}
	query := filterQuery(filter)
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return nil, err
	}
	cursor, err := collection.Find(ctx, query)
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &jobs)
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
		envVar := e.EnvVar
		envVar.ManagedBy = fmt.Sprintf("%s/%s", e.ServiceName, e.InstanceName)
		mergedEnvs[e.Name] = envVar
	}
	sort.Strings(toInterpolateKeys)

	for _, envName := range toInterpolateKeys {
		tsuruEnvs.Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}

	mergedEnvs[tsuruEnvs.TsuruServicesEnvVar] = tsuruEnvs.ServiceEnvsFromEnvVars(job.Spec.ServiceEnvs)

	return mergedEnvs
}

func SetEnvs(ctx context.Context, job *jobTypes.Job, setEnvs bindTypes.SetEnvArgs) error {
	if setEnvs.ManagedBy == "" && len(setEnvs.Envs) == 0 {
		return nil
	}

	if setEnvs.Writer != nil && len(setEnvs.Envs) > 0 {
		streamfmt.FprintlnSectionf(setEnvs.Writer, "Setting %d new environment variables", len(setEnvs.Envs))
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
					streamfmt.FprintlnSectionf(setEnvs.Writer, "Pruning %s from environment variables", env.Name)
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

	if !shouldUpdateJobProvision(job) {
		return nil
	}

	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return err
	}
	return prov.EnsureJob(ctx, job)

}

func UnsetEnvs(ctx context.Context, job *jobTypes.Job, unsetEnvs bindTypes.UnsetEnvArgs) error {
	if len(unsetEnvs.VariableNames) == 0 {
		return nil
	}
	if unsetEnvs.Writer != nil {
		streamfmt.FprintlnSectionf(unsetEnvs.Writer, "Unsetting %d environment variables", len(unsetEnvs.VariableNames))
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

	if !shouldUpdateJobProvision(job) {
		return nil
	}

	prov, err := getProvisioner(ctx, job)
	if err != nil {
		return err
	}
	return prov.EnsureJob(ctx, job)
}

func shouldUpdateJobProvision(job *jobTypes.Job) bool {
	//no need to update provisioner if there is no image provided yet
	return job.Spec.Container.InternalRegistryImage != "" || job.Spec.Container.OriginalImageSrc != ""
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
	plans, err := pool.GetPlans(ctx)
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
	poolTeams, err := p.GetTeams(ctx)
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
	if j.Spec.ConcurrencyPolicy != nil {
		allowedValues := []string{"Allow", "Forbid", "Replace"}
		if !set.FromSlice(allowedValues).Includes(*j.Spec.ConcurrencyPolicy) {
			return &tsuruErrors.ValidationError{Message: jobTypes.ErrInvalidConcurrencyPolicy.Error()}
		}
	}
	return nil
}
