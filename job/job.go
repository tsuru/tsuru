// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.

package job

import (
	"context"
	"time"

	"github.com/globalsign/mgo"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	"gopkg.in/mgo.v2/bson"
)

type Job struct {
	Name        string
	Teams       []string
	TeamOwner   string
	Owner       string
	Plan        appTypes.Plan
	Metadata    appTypes.Metadata
	Pool        string
	Description string

	AttemptedRuns uint
	Completions   uint
	Cron          bool
	Schedule      map[string]string

	CreatedAt *time.Time `bson:"createdAt"`

	ctx         context.Context
	provisioner provision.Provisioner
}

func (job *Job) getProvisioner() (provision.Provisioner, error) {
	var err error
	if job.provisioner == nil {
		job.provisioner, err = pool.GetProvisionerForPool(job.ctx, job.Pool)
	}
	return job.provisioner, err
}

// Units returns the list of units.
func (job *Job) Units() ([]provision.JobUnit, error) {
	prov, err := job.getProvisioner()
	if err != nil {
		return []provision.JobUnit{}, err
	}
	return prov.JobUnits(context.TODO(), job)
}

func (job *Job) GetName() string {
	return job.Name
}

// GetExecutions returns a pair of attempted runs followed by it's successfull runs: {No of attempts, No of successfull runs}
func (job *Job) GetExecutions() []uint {
	return []uint{job.AttemptedRuns, job.Completions}
}

func (job *Job) Envs() map[string]bind.EnvVar {
	return nil
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
	return job.Cron
}

// GetJobByName queries the database to find a job identified by the given
// name.
func GetJobByName(ctx context.Context, name string) (*Job, error) {
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
	job.ctx = ctx
	return &job, err
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
	if job.Cron {
		actions = []*action.Action{
			&reserveTeamCronjob,
			&reserveUserCronjob,
			&insertJob,
			&provisionJob,
		}
	} else {
		actions = []*action.Action{
			&insertJob,
			&provisionJob,
		}
	}

	pipeline := action.NewPipeline(actions...)
	err := pipeline.Execute(ctx, job, user)
	if err != nil {
		return err
	}
	return nil
}

var provisionJob = action.Action{
	Name: "provision-job",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var job *Job
		switch ctx.Params[0].(type) {
		case *Job:
			job = ctx.Params[0].(*Job)
		default:
			return nil, errors.New("First parameter must be *Job.")
		}
		prov, err := job.getProvisioner()
		if err != nil {
			return nil, err
		}
		if err = prov.CreateJob(ctx.Context, job); err != nil {
			return nil, err
		}
		return job, nil
	},
	Backward: func(ctx action.BWContext) {
		job := ctx.FWResult.(*Job)
		prov, err := job.getProvisioner()
		if err == nil {
			prov.DestroyJob(ctx.Context, job)
		}
	},
	MinParams: 1,
}
