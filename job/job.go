// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.

package job

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	"gopkg.in/mgo.v2/bson"
)

type Job struct {
	TsuruJob
	// Specifies the maximum desired number of pods the job should
	// run at any given time. The actual number of pods running in steady state will
	// be less than this number when ((.spec.completions - .status.successful) < .spec.parallelism),
	// i.e. when the work left to do is less than max parallelism.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	// +optional
	Parallelism *int32 `json:"parallelism,omitempty" protobuf:"varint,1,opt,name=parallelism"`

	// Specifies the desired number of successfully finished pods the
	// job should be run with.  Setting to nil means that the success of any
	// pod signals the success of all pods, and allows parallelism to have any positive
	// value.  Setting to 1 means that parallelism is limited to 1 and the success of that
	// pod signals the success of the job.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	// +optional
	Completions *int32 `json:"completions,omitempty" protobuf:"varint,2,opt,name=completions"`

	// Specifies the duration in seconds relative to the startTime that the job may be active
	// before the system tries to terminate it; value must be positive integer
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty" protobuf:"varint,3,opt,name=activeDeadlineSeconds"`

	// Specifies the number of retries before marking this job failed.
	// Defaults to 6
	// +optional
	BackoffLimit *int32 `json:"backoffLimit,omitempty" protobuf:"varint,7,opt,name=backoffLimit"`

	Schedule string

	Containers []jobTypes.ContainerInfo
}

type miniJob struct {
	Name      string
	Pool      string
	TeamOwner string
	Units     []provision.Unit
}

type TsuruJob struct {
	Name        string
	Teams       []string
	TeamOwner   string
	Owner       string
	Plan        appTypes.Plan
	Metadata    appTypes.Metadata
	Pool        string
	Description string

	ctx         context.Context
	provisioner provision.JobProvisioner
}

func (job *Job) getProvisioner() (provision.JobProvisioner, error) {
	var err error
	if job.provisioner != nil {
		return job.provisioner, nil
	}
	prov, err := pool.GetProvisionerForPool(job.ctx, job.Pool)
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
func (job *Job) Units() ([]provision.Unit, error) {
	prov, err := job.getProvisioner()
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

// GetSwap returns the swap limit (in bytes) for the job.
func (job *Job) GetSwap() int64 {
	return job.Plan.Swap
}

// GetCpuShare returns the cpu share for the job.
func (job *Job) GetCpuShare() int {
	return job.Plan.CpuShare
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
	return job.Schedule != ""
}

func (job *Job) GetContainersInfo() []jobTypes.ContainerInfo {
	return job.Containers
}

func (job *Job) GetSchedule() string {
	return job.Schedule
}

// GetByName queries the database to find a job identified by the given
// name.
func GetByNameAndTeam(ctx context.Context, name, teamOwner string) (*Job, error) {
	var job Job
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Jobs().Find(bson.M{"tsurujob.name": name, "tsurujob.teamowner": teamOwner}).One(&job)
	if err == mgo.ErrNotFound {
		return nil, jobTypes.ErrJobNotFound
	}
	job.ctx = ctx
	return &job, err
}

func RemoveJobFromDb(jobName, jobTeamOwner string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Jobs().Remove(bson.M{"tsurujob.name": jobName, "tsurujob.teamowner": jobTeamOwner})
	if err == mgo.ErrNotFound {
		return jobTypes.ErrJobNotFound
	}
	return err
}

func DeleteFromProvisioner(ctx context.Context, job *Job) error {
	prov, err := job.getProvisioner()
	if err != nil {
		return err
	}
	return prov.DestroyJob(job.ctx, job)
}

// CreateJob creates a new job or cronjob.
//
// Creating a new job is a process composed of the following steps:
//
//  1. Save the job in the database
//  2. Provision the job using the provisioner
func CreateJob(ctx context.Context, job *Job, user *auth.User) error {
	if job.ctx == nil {
		job.ctx = ctx
	}
	if err := buildName(ctx, job); err != nil {
		return err
	}
	if err := buildPlan(ctx, job); err != nil {
		return err
	}
	buildTsuruInfo(ctx, job, user)

	var actions []*action.Action
	if job.IsCron() {
		actions = []*action.Action{
			&reserveTeamCronjob,
			&reserveUserCronjob,
			&provisionJob,
			&insertJob,
		}
	} else {
		actions = []*action.Action{
			&provisionJob,
			&insertJob,
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
func UpdateJob(ctx context.Context, job *Job, user *auth.User) error {
	pipeline := action.NewPipeline([]*action.Action{
		&updateJobProv,
		&jobUpdateDB,
	}...)
	return pipeline.Execute(ctx, job, user)
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
		query["tsurujob.name"] = bson.M{"$regex": f.Name}
	}
	if f.TeamOwner != "" {
		query["tsurujob.teamowner"] = f.TeamOwner
	}
	if f.UserOwner != "" {
		query["tsurujob.owner"] = f.UserOwner
	}
	if f.Pool != "" {
		query["tsurujob.pool"] = f.Pool
	}
	if len(f.Pools) > 0 {
		query["tsurujob.pool"] = bson.M{"$in": f.Pools}
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
