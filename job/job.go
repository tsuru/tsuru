// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.

package job

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrJobNotFound        = errors.New("Job not found")
	maxAttempts           = 5
	ErrMaxAttemptsReached = errors.New(fmt.Sprintf("Unable to generate unique job name: max attempts reached (%d)", maxAttempts))
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
	IsCron        bool
	Schedule      map[string]string

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
	units, err := prov.JobUnits(context.TODO(), job)
	if units == nil {
		// This is unusual but was done because previously this method didn't
		// return an error. This ensures we always return an empty list instead
		// of nil to preserve compatibility with old clients.
		units = []provision.JobUnit{}
	}
	return units, err
}

func (job *Job) GetName() string {
	return job.Name
}

// GetExecutions returns the a pair of attempted runs followed by it's successfull runs.
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

// GetJObByName queries the database to find a job identified by the given
// name.
func GetJobByName(ctx context.Context, name string) (*Job, error) {
	var job Job
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Apps().Find(bson.M{"name": name}).One(&job)
	if err == mgo.ErrNotFound {
		return nil, ErrJobNotFound
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
	buildOwnerInfo(ctx, job, user)
	return nil
}
