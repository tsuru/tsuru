// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"encoding/json"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type mongoOperation string

var (
	mongoSpanUpdateAll mongoOperation = "UpdateAll"
	mongoSpanUpsert    mongoOperation = "Upsert"
	mongoSpanUpsertID  mongoOperation = "UpsertID"
	mongoSpanFind      mongoOperation = "Find"
	mongoSpanFindOne   mongoOperation = "FindOne"
	mongoSpanFindID    mongoOperation = "FindID"
	mongoSpanDelete    mongoOperation = "Delete"
	mongoSpanDeleteID  mongoOperation = "DeleteID"
	mongoSpanInsert    mongoOperation = "Insert"
	mongoSpanUpdate    mongoOperation = "Update"
	mongoSpanUpdateID  mongoOperation = "UpdateID"
)

var tracer = otel.Tracer("tsuru/storage/mongodb")

type mongoDBSpan struct {
	trace.Span
}

func newMongoDBSpan(ctx context.Context, operation mongoOperation, collection string) *mongoDBSpan {
	if ctx == nil {
		ctx = context.Background()
	}

	spanName := string(operation) + " " + collection
	_, span := tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("component", "mongodb"),
			attribute.String("db.type", "mongodb"),
			attribute.String("db.operation", string(operation)),
			attribute.String("db.collection", collection),
		),
	)

	return &mongoDBSpan{span}
}

// Finish is a compatibility method that calls End()
func (s *mongoDBSpan) Finish() {
	s.End()
}

// LogKV is a compatibility method for old logging API
func (s *mongoDBSpan) LogKV(keyvals ...interface{}) {
	// OpenTelemetry doesn't have direct equivalent, ignoring
}

func (s *mongoDBSpan) SetQueryStatement(query interface{}) {
	value, _ := json.Marshal(query)
	s.SetAttributes(attribute.String("db.statement", string(value)))
}

func (s *mongoDBSpan) SetMongoID(id interface{}) {
	s.SetQueryStatement(mongoBSON.M{"_id": id})
}

func (s *mongoDBSpan) SetError(err error) {
	if err == nil {
		return
	}
	s.SetStatus(codes.Error, err.Error())
	s.RecordError(err)
}
