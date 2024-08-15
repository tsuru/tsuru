// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"strings"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/event"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type webhookStorage struct{}

var _ event.WebhookStorage = &webhookStorage{}

func (s *webhookStorage) Insert(ctx context.Context, w event.Webhook) error {
	collection, err := storagev2.WebhookCollection()
	if err != nil {
		return err
	}
	_, err = collection.InsertOne(ctx, w)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		err = event.ErrWebhookAlreadyExists
	}
	return err
}

func (s *webhookStorage) Update(ctx context.Context, w event.Webhook) error {
	collection, err := storagev2.WebhookCollection()
	if err != nil {
		return err
	}
	result, err := collection.ReplaceOne(ctx, mongoBSON.M{"name": w.Name}, w)
	if err == mongo.ErrNoDocuments {
		return event.ErrWebhookNotFound
	}

	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return event.ErrWebhookNotFound
	}

	return err
}

func (s *webhookStorage) findQuery(ctx context.Context, query mongoBSON.M) ([]event.Webhook, error) {
	collection, err := storagev2.WebhookCollection()
	if err != nil {
		return nil, err
	}

	cursor, err := collection.Find(ctx, query)
	if err != nil {
		return nil, err
	}

	var webhooks []event.Webhook
	err = cursor.All(ctx, &webhooks)
	if err != nil {
		return nil, err
	}
	return webhooks, nil
}

func (s *webhookStorage) FindAllByTeams(ctx context.Context, teams []string) ([]event.Webhook, error) {
	var query mongoBSON.M
	if teams != nil {
		query = mongoBSON.M{"teamowner": mongoBSON.M{"$in": teams}}
	}
	return s.findQuery(ctx, query)
}

func (s *webhookStorage) FindByEvent(ctx context.Context, f event.WebhookEventFilter, isSuccess bool) ([]event.Webhook, error) {
	for _, name := range f.KindNames {
		parts := strings.Split(name, ".")
		parts = parts[:len(parts)-1]
		for i := 1; i < len(parts); i++ {
			parts[i] = parts[i-1] + "." + parts[i]
		}
		f.KindNames = append(f.KindNames, parts...)
	}
	andBlock := []mongoBSON.M{
		{"$or": []mongoBSON.M{{"eventfilter.targettypes": mongoBSON.M{"$in": f.TargetTypes}}, {"eventfilter.targettypes": []string{}}}},
		{"$or": []mongoBSON.M{{"eventfilter.targetvalues": mongoBSON.M{"$in": f.TargetValues}}, {"eventfilter.targetvalues": []string{}}}},
		{"$or": []mongoBSON.M{{"eventfilter.kindtypes": mongoBSON.M{"$in": f.KindTypes}}, {"eventfilter.kindtypes": []string{}}}},
		{"$or": []mongoBSON.M{{"eventfilter.kindnames": mongoBSON.M{"$in": f.KindNames}}, {"eventfilter.kindnames": []string{}}}},
	}
	if isSuccess {
		andBlock = append(andBlock, mongoBSON.M{"eventfilter.erroronly": false})
	} else {
		andBlock = append(andBlock, mongoBSON.M{"eventfilter.successonly": false})
	}
	return s.findQuery(ctx, mongoBSON.M{"$and": andBlock})
}

func (s *webhookStorage) FindByName(ctx context.Context, name string) (*event.Webhook, error) {
	collection, err := storagev2.WebhookCollection()
	if err != nil {
		return nil, err
	}

	var result event.Webhook

	err = collection.FindOne(ctx, mongoBSON.M{"name": name}).Decode(&result)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			err = event.ErrWebhookNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (s *webhookStorage) Delete(ctx context.Context, name string) error {
	collection, err := storagev2.WebhookCollection()
	if err != nil {
		return err
	}
	result, err := collection.DeleteOne(ctx, mongoBSON.M{"name": name})
	if err == mongo.ErrNoDocuments {
		return event.ErrWebhookNotFound
	}
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return event.ErrWebhookNotFound
	}

	return nil
}
