// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package db encapsulates tsuru connection with MongoDB.
//
// The function Conn dials to MongoDB using data from the configuration file
// and returns a connection (represented by the storage.Storage type). It
// manages an internal pool of connections, and reconnects in case of failures.
// That means that you should not store references to the connection, but
// always call Open.
package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/globalsign/mgo"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/hc"
)

const (
	DefaultDatabaseURL  = "127.0.0.1:27017"
	DefaultDatabaseName = "tsuru"
)

type Storage struct {
	*storage.Storage
}

func init() {
	hc.AddChecker("MongoDB", healthCheck)
}

func healthCheck(ctx context.Context) error {
	conn, err := Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Database.Session.Ping()
}

func DbConfig(prefix string) (string, string) {
	url, _ := config.GetString(fmt.Sprintf("database:%surl", prefix))
	if url == "" {
		url, _ = config.GetString("database:url")
		if url == "" {
			url = DefaultDatabaseURL
		}
	}
	dbname, _ := config.GetString(fmt.Sprintf("database:%sname", prefix))
	if dbname == "" {
		dbname, _ = config.GetString("database:name")
		if dbname == "" {
			dbname = DefaultDatabaseName
		}
	}
	return url, dbname
}

// Conn reads the tsuru config and calls storage.Open to get a database connection.
//
// Most tsuru packages should probably use this function. storage.Open is intended for
// use when supporting more than one database.
func Conn() (*Storage, error) {
	var (
		strg Storage
		err  error
	)
	url, dbname := DbConfig("")
	strg.Storage, err = storage.Open(url, dbname)
	return &strg, err
}

// Apps returns the apps collection from MongoDB.
func (s *Storage) Apps() *storage.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.Collection("apps")
	c.EnsureIndex(nameIndex)
	return c
}

func IsCollectionExistsError(err error) bool {
	if err == nil {
		return false
	}
	if queryErr, ok := err.(*mgo.QueryError); ok && queryErr.Code == 48 {
		return true
	}
	return strings.Contains(err.Error(), "already exists")
}
