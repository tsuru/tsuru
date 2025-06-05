// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// newMongoDBOperationSpan starts a new OpenTelemetry span for a MongoDB operation.
// It's the caller's responsibility to call span.End().
func newMongoDBOperationSpan(ctx context.Context, operationName string, collectionName string) (context.Context, trace.Span) {
	tracer := otel.Tracer("tsuru/storage/mongodb")
	ctx, span := tracer.Start(ctx, operationName+" "+collectionName, trace.WithSpanKind(trace.SpanKindClient))

	span.SetAttributes(
		semconv.DBSystemMongoDB,
		semconv.DBNameKey.String("tsuru"), // Assuming "tsuru" is the database name. This might need to be dynamic.
		semconv.DBMongoDBCollectionKey.String(collectionName),
	)
	// The otelmongo.NewMonitor() will add more detailed information, including the operation itself.
	// This span provides a higher-level view of the operation within the tsuru codebase.
	return ctx, span
}

// SetQueryStatement sets the database statement attribute on the span.
func SetQueryStatement(span trace.Span, query interface{}) {
	if query == nil {
		return
	}
	value, err := json.Marshal(query)
	if err == nil {
		span.SetAttributes(semconv.DBStatementKey.String(string(value)))
	}
}

// SetMongoID sets the MongoDB document ID as a query statement attribute on the span.
func SetMongoID(span trace.Span, id interface{}) {
	if id == nil {
		return
	}
	SetQueryStatement(span, mongoBSON.M{"_id": id})
}

// SetError records an error on the span.
func SetError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// Ensure client is initialized with otelmongo.NewMonitor() where mongo.Connect or mongo.NewClient is called.
// Example (should be in the client initialization code, not here):
//
// import (
// 	"go.mongodb.org/mongo-driver/mongo"
// 	"go.mongodb.org/mongo-driver/mongo/options"
// 	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
// )
//
// func connectToMongoDB() (*mongo.Client, error) {
// 	clientOptions := options.Client().ApplyURI("<your-mongo-uri>")
// 	clientOptions.SetMonitor(otelmongo.NewMonitor())
// 	client, err := mongo.Connect(context.Background(), clientOptions)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return client, nil
// }
