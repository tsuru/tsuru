// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tsuru/tsuru/db/storagepg"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
)

type JobStorage struct{}

var _ jobTypes.JobStorage = &JobStorage{}

const uniqueViolation = "23505"

// jobDoc is the on-disk representation. It carries fields the public API hides
// from JSON (Spec.ServiceEnvs is tagged `json:"-"`) but that storage must
// persist, keeping the public API contract untouched.
type jobDoc struct {
	jobTypes.Job
	ServiceEnvs []serviceEnvVar `json:"_serviceEnvs"`
}

// serviceEnvVar mirrors bindTypes.ServiceEnvVar with storage JSON tags:
// ServiceName and InstanceName are `json:"-"` on the public type.
type serviceEnvVar struct {
	bindTypes.EnvVar
	ServiceName  string `json:"serviceName"`
	InstanceName string `json:"instanceName"`
}

func toServiceEnvs(envs []bindTypes.ServiceEnvVar) []serviceEnvVar {
	if envs == nil {
		return nil
	}
	out := make([]serviceEnvVar, len(envs))
	for i, e := range envs {
		out[i] = serviceEnvVar{EnvVar: e.EnvVar, ServiceName: e.ServiceName, InstanceName: e.InstanceName}
	}
	return out
}

func fromServiceEnvs(envs []serviceEnvVar) []bindTypes.ServiceEnvVar {
	if envs == nil {
		return nil
	}
	out := make([]bindTypes.ServiceEnvVar, len(envs))
	for i, e := range envs {
		out[i] = bindTypes.ServiceEnvVar{EnvVar: e.EnvVar, ServiceName: e.ServiceName, InstanceName: e.InstanceName}
	}
	return out
}

func marshalJob(j jobTypes.Job) ([]byte, error) {
	return json.Marshal(jobDoc{Job: j, ServiceEnvs: toServiceEnvs(j.Spec.ServiceEnvs)})
}

func unmarshalJob(data []byte) (jobTypes.Job, error) {
	var d jobDoc
	if err := json.Unmarshal(data, &d); err != nil {
		return jobTypes.Job{}, err
	}
	job := d.Job
	job.Spec.ServiceEnvs = fromServiceEnvs(d.ServiceEnvs)
	return job, nil
}

func (s *JobStorage) Insert(ctx context.Context, job jobTypes.Job) error {
	p, err := storagepg.Pool()
	if err != nil {
		return err
	}
	doc, err := marshalJob(job)
	if err != nil {
		return err
	}
	_, err = p.Exec(ctx,
		`INSERT INTO jobs (name, team_owner, owner, pool, tags, doc)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		job.Name, job.TeamOwner, job.Owner, job.Pool, job.Tags, doc)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return jobTypes.ErrJobAlreadyExists
	}
	return err
}

func (s *JobStorage) Update(ctx context.Context, job jobTypes.Job) error {
	p, err := storagepg.Pool()
	if err != nil {
		return err
	}
	doc, err := marshalJob(job)
	if err != nil {
		return err
	}
	tag, err := p.Exec(ctx,
		`UPDATE jobs SET team_owner=$2, owner=$3, pool=$4, tags=$5, doc=$6
		 WHERE name=$1`,
		job.Name, job.TeamOwner, job.Owner, job.Pool, job.Tags, doc)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return jobTypes.ErrJobNotFound
	}
	return nil
}

func (s *JobStorage) FindByName(ctx context.Context, name string) (*jobTypes.Job, error) {
	p, err := storagepg.Pool()
	if err != nil {
		return nil, err
	}
	var doc []byte
	err = p.QueryRow(ctx, `SELECT doc FROM jobs WHERE name=$1`, name).Scan(&doc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, jobTypes.ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	job, err := unmarshalJob(doc)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *JobStorage) ListByFilter(ctx context.Context, filter *jobTypes.Filter) ([]jobTypes.Job, error) {
	p, err := storagepg.Pool()
	if err != nil {
		return nil, err
	}
	where, args := translateJobQuery(filter.ToQuery())
	sql := `SELECT doc FROM jobs`
	if where != "" {
		sql += " WHERE " + where
	}
	rows, err := p.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []jobTypes.Job{}
	for rows.Next() {
		var doc []byte
		if err = rows.Scan(&doc); err != nil {
			return nil, err
		}
		job, err := unmarshalJob(doc)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *JobStorage) Delete(ctx context.Context, name string) error {
	p, err := storagepg.Pool()
	if err != nil {
		return err
	}
	tag, err := p.Exec(ctx, `DELETE FROM jobs WHERE name=$1`, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return jobTypes.ErrJobNotFound
	}
	return nil
}

// UpdateServiceEnvs is a read-modify-write here (vs Mongo's `$set`). If Job ever
// needs concurrency-safe partial updates, switch to a jsonb_set UPDATE.
func (s *JobStorage) UpdateServiceEnvs(ctx context.Context, name string, serviceEnvs []bindTypes.ServiceEnvVar) error {
	job, err := s.FindByName(ctx, name)
	if err != nil {
		return err
	}
	job.Spec.ServiceEnvs = serviceEnvs
	return s.Update(ctx, *job)
}
