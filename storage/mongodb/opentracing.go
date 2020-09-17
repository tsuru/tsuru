// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"encoding/json"

	"github.com/opentracing/opentracing-go"
	opentracingExt "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"gopkg.in/mgo.v2/bson"
)

type mongoOperation string

var (
	mongoSpanUpdateAll mongoOperation = "UpdateAll"
	mongoSpanUpsert    mongoOperation = "Upsert"
	mongoSpanFind      mongoOperation = "Find"
	mongoSpanFindOne   mongoOperation = "FindOne"
	mongoSpanFindID    mongoOperation = "FindID"
	mongoSpanDelete    mongoOperation = "Delete"
	mongoSpanInsert    mongoOperation = "Insert"
)

var (
	opentracingComponent = opentracing.Tag{Key: "component", Value: "mongodb"}
	opentracingDBType    = opentracing.Tag{Key: "db.type", Value: "mongodb"}
)

type mongoDBSpan struct {
	opentracing.Span
}

func newMongoDBSpan(ctx context.Context, operation mongoOperation, collection string) *mongoDBSpan {
	options := []opentracing.StartSpanOption{
		opentracingExt.SpanKindRPCClient,
		opentracingComponent,
		opentracingDBType,
	}
	span, _ := opentracing.StartSpanFromContext(
		ctx, string(operation)+" "+collection,
		options...,
	)

	return &mongoDBSpan{span}
}

func (s *mongoDBSpan) SetQueryStatement(query interface{}) {
	value, _ := json.Marshal(query)
	s.SetTag(string(opentracingExt.DBStatement), string(value))
}

func (s *mongoDBSpan) SetMongoID(id interface{}) {
	s.SetQueryStatement(bson.M{"_id": id})
}

func (s *mongoDBSpan) SetError(err error) {
	if err == nil {
		return
	}
	opentracingExt.Error.Set(s, true)
	s.LogFields(
		log.String("event", "error"),
		log.String("error.object", err.Error()),
	)
}
