// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	storagev2 "github.com/tsuru/tsuru/db/storagev2"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type JobStorage struct{}

var _ jobTypes.JobStorage = &JobStorage{}

func (s *JobStorage) Insert(ctx context.Context, job jobTypes.Job) error {
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return err
	}
	span := newMongoDBSpan(ctx, mongoSpanInsert, collection.Name())
	defer span.Finish()

	_, err = collection.InsertOne(ctx, job)
	if mongo.IsDuplicateKeyError(err) {
		err = jobTypes.ErrJobAlreadyExists
	}
	span.SetError(err)
	return err
}

func (s *JobStorage) Update(ctx context.Context, job jobTypes.Job) error {
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return err
	}
	span := newMongoDBSpan(ctx, mongoSpanUpdate, collection.Name())
	defer span.Finish()

	result, err := collection.ReplaceOne(ctx, mongoBSON.M{"name": job.Name}, job)
	if err != nil {
		span.SetError(err)
		return err
	}
	if result.MatchedCount == 0 {
		return jobTypes.ErrJobNotFound
	}
	return nil
}

func (s *JobStorage) FindByName(ctx context.Context, name string) (*jobTypes.Job, error) {
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return nil, err
	}
	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	defer span.Finish()

	var job jobTypes.Job
	err = collection.FindOne(ctx, mongoBSON.M{"name": name}).Decode(&job)
	if err == mongo.ErrNoDocuments {
		return nil, jobTypes.ErrJobNotFound
	}
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	return &job, nil
}

func (s *JobStorage) ListByFilter(ctx context.Context, filter *jobTypes.Filter) ([]jobTypes.Job, error) {
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return nil, err
	}
	query := translateQuery(filter.ToQuery())

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

	jobs := []jobTypes.Job{}
	cursor, err := collection.Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	if err = cursor.All(ctx, &jobs); err != nil {
		span.SetError(err)
		return nil, err
	}
	return jobs, nil
}

func (s *JobStorage) Delete(ctx context.Context, name string) error {
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return err
	}
	span := newMongoDBSpan(ctx, mongoSpanDelete, collection.Name())
	defer span.Finish()

	result, err := collection.DeleteOne(ctx, mongoBSON.M{"name": name})
	if err != nil {
		span.SetError(err)
		return err
	}
	if result.DeletedCount == 0 {
		return jobTypes.ErrJobNotFound
	}
	return nil
}

func (s *JobStorage) UpdateServiceEnvs(ctx context.Context, name string, serviceEnvs []bindTypes.ServiceEnvVar) error {
	collection, err := storagev2.JobsCollection()
	if err != nil {
		return err
	}
	span := newMongoDBSpan(ctx, mongoSpanUpdate, collection.Name())
	defer span.Finish()

	result, err := collection.UpdateOne(ctx,
		mongoBSON.M{"name": name},
		mongoBSON.M{"$set": mongoBSON.M{"spec.serviceenvs": serviceEnvs}},
	)
	if err != nil {
		span.SetError(err)
		return err
	}
	if result.MatchedCount == 0 {
		return jobTypes.ErrJobNotFound
	}
	return nil
}
