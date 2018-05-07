// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"strings"

	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/event"
)

type webhookStorage struct{}

func webhookCollection(conn *db.Storage) *dbStorage.Collection {
	coll := conn.Collection("webhook")
	coll.EnsureIndex(mgo.Index{
		Key:    []string{"name"},
		Unique: true,
	})
	return coll
}

var _ event.WebhookStorage = &webhookStorage{}

func (s *webhookStorage) Insert(w event.Webhook) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = webhookCollection(conn).Insert(w)
	if err != nil && mgo.IsDup(err) {
		err = event.ErrWebhookAlreadyExists
	}
	return err
}

func (s *webhookStorage) Update(w event.Webhook) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = webhookCollection(conn).Update(bson.M{"name": w.Name}, w)
	if err == mgo.ErrNotFound {
		err = event.ErrWebhookNotFound
	}
	return err
}

func (s *webhookStorage) findQuery(query bson.M) ([]event.Webhook, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var webhooks []event.Webhook
	err = webhookCollection(conn).Find(query).All(&webhooks)
	return webhooks, err
}

func (s *webhookStorage) FindAllByTeams(teams []string) ([]event.Webhook, error) {
	var query bson.M
	if teams != nil {
		query = bson.M{"teamowner": bson.M{"$in": teams}}
	}
	return s.findQuery(query)
}

func (s *webhookStorage) FindByEvent(f event.WebhookEventFilter, isSuccess bool) ([]event.Webhook, error) {
	for _, name := range f.KindNames {
		parts := strings.Split(name, ".")
		parts = parts[:len(parts)-1]
		for i := 1; i < len(parts); i++ {
			parts[i] = parts[i-1] + "." + parts[i]
		}
		f.KindNames = append(f.KindNames, parts...)
	}
	andBlock := []bson.M{
		{"$or": []bson.M{{"eventfilter.targettypes": bson.M{"$in": f.TargetTypes}}, {"eventfilter.targettypes": []string{}}}},
		{"$or": []bson.M{{"eventfilter.targetvalues": bson.M{"$in": f.TargetValues}}, {"eventfilter.targetvalues": []string{}}}},
		{"$or": []bson.M{{"eventfilter.kindtypes": bson.M{"$in": f.KindTypes}}, {"eventfilter.kindtypes": []string{}}}},
		{"$or": []bson.M{{"eventfilter.kindnames": bson.M{"$in": f.KindNames}}, {"eventfilter.kindnames": []string{}}}},
	}
	if isSuccess {
		andBlock = append(andBlock, bson.M{"eventfilter.erroronly": false})
	} else {
		andBlock = append(andBlock, bson.M{"eventfilter.successonly": false})
	}
	return s.findQuery(bson.M{"$and": andBlock})
}

func (s *webhookStorage) FindByName(name string) (*event.Webhook, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var result event.Webhook
	err = webhookCollection(conn).Find(bson.M{"name": name}).One(&result)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = event.ErrWebhookNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (s *webhookStorage) Delete(name string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = webhookCollection(conn).Remove(bson.M{"name": name})
	if err == mgo.ErrNotFound {
		err = event.ErrWebhookNotFound
	}
	return err
}
