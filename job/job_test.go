package job

import (
	"context"

	"gopkg.in/check.v1"
)

func (s *S) TestGetByName(c *check.C) {
	newJob := Job{
		TsuruJob: TsuruJob{
			Name:      "some-job",
			TeamOwner: s.team.Name,
			Pool:      s.Pool,
			Teams:     []string{s.team.Name},
		},
	}
	err := CreateJob(context.TODO(), &newJob, s.user, false)
	c.Assert(err, check.IsNil)
	myJob, err := GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newJob.Name, check.DeepEquals, myJob.Name)
}
