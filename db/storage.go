// Copyright 2015 tsuru authors. All rights reserved.
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
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/hc"
	"gopkg.in/mgo.v2"
)

const (
	DefaultDatabaseURL  = "127.0.0.1:27017"
	DefaultDatabaseName = "tsuru"
)

type Storage struct {
	*storage.Storage
}

type LogStorage struct {
	*storage.Storage
}

func init() {
	hc.AddChecker("MongoDB", healthCheck)
}

func healthCheck() error {
	conn, err := Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Database.Session.Ping()
}

func dbConfig(prefix string) (string, string) {
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
	url, dbname := dbConfig("")
	strg.Storage, err = storage.Open(url, dbname)
	return &strg, err
}

func LogConn() (*LogStorage, error) {
	var (
		strg LogStorage
		err  error
	)
	url, dbname := dbConfig("logdb-")
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

func (s *Storage) Deploys() *storage.Collection {
	timestampIndex := mgo.Index{Key: []string{"-timestamp"}}
	c := s.Collection("deploys")
	c.EnsureIndex(timestampIndex)
	return c
}

// Platforms returns the platforms collection from MongoDB.
func (s *Storage) Platforms() *storage.Collection {
	return s.Collection("platforms")
}

// Services returns the services collection from MongoDB.
func (s *Storage) Services() *storage.Collection {
	return s.Collection("services")
}

// ServiceInstances returns the services_instances collection from MongoDB.
func (s *Storage) ServiceInstances() *storage.Collection {
	return s.Collection("service_instances")
}

// Plans returns the plans collection.
func (s *Storage) Plans() *storage.Collection {
	return s.Collection("plans")
}

// Users returns the users collection from MongoDB.
func (s *Storage) Users() *storage.Collection {
	emailIndex := mgo.Index{Key: []string{"email"}, Unique: true}
	c := s.Collection("users")
	c.EnsureIndex(emailIndex)
	return c
}

func (s *Storage) Tokens() *storage.Collection {
	coll := s.Collection("tokens")
	coll.EnsureIndex(mgo.Index{Key: []string{"token"}})
	return coll
}

func (s *Storage) PasswordTokens() *storage.Collection {
	return s.Collection("password_tokens")
}

func (s *Storage) UserActions() *storage.Collection {
	return s.Collection("user_actions")
}

// Teams returns the teams collection from MongoDB.
func (s *Storage) Teams() *storage.Collection {
	return s.Collection("teams")
}

// Quota returns the quota collection from MongoDB.
func (s *Storage) Quota() *storage.Collection {
	userIndex := mgo.Index{Key: []string{"owner"}, Unique: true}
	c := s.Collection("quota")
	c.EnsureIndex(userIndex)
	return c
}

var logCappedInfo = mgo.CollectionInfo{
	Capped:       true,
	MaxBytes:     200 * 5000,
	MaxDocs:      5000,
	ForceIdIndex: true,
}

// Logs returns the logs collection for one app from MongoDB.
func (s *LogStorage) Logs(appName string) *storage.Collection {
	if appName == "" {
		return nil
	}
	c := s.Collection("logs_" + appName)
	c.Create(&logCappedInfo)
	return c
}

// LogsCollections returns logs collections for all apps from MongoDB.
func (s *LogStorage) LogsCollections() ([]*storage.Collection, error) {
	var names []struct {
		Name string
	}
	conn, err := Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Apps().Find(nil).All(&names)
	if err != nil {
		return nil, err
	}
	var colls []*storage.Collection
	for _, name := range names {
		colls = append(colls, s.Collection("logs_"+name.Name))
	}
	return colls, nil
}
