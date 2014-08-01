// Copyright 2014 tsuru authors. All rights reserved.
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
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2"
)

const (
	DefaultDatabaseURL  = "127.0.0.1:27017"
	DefaultDatabaseName = "tsuru"
)

type Storage struct {
	*storage.Storage
}

// conn reads the tsuru config and calls storage.Open to get a database connection.
//
// Most tsuru packages should probably use this function. storage.Open is intended for
// use when supporting more than one database.
func conn() (*storage.Storage, error) {
	url, _ := config.GetString("database:url")
	if url == "" {
		url = DefaultDatabaseURL
	}
	dbname, _ := config.GetString("database:name")
	if dbname == "" {
		dbname = DefaultDatabaseName
	}
	return storage.Open(url, dbname)
}

func Conn() (*Storage, error) {
	var (
		strg Storage
		err  error
	)
	strg.Storage, err = conn()
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
	return s.Collection("deploys")
}

// Platforms returns the platforms collection from MongoDB.
func (s *Storage) Platforms() *storage.Collection {
	return s.Collection("platforms")
}

// Logs returns the logs collection from MongoDB.
func (s *Storage) Logs(appName string) *storage.Collection {
	if appName == "" {
		return nil
	}
	sourceIndex := mgo.Index{Key: []string{"source"}}
	unitIndex := mgo.Index{Key: []string{"unit"}}
	c := s.Collection("logs_" + appName)
	meanSize := 200
	maxLines := 5000
	info := mgo.CollectionInfo{Capped: true, MaxBytes: meanSize * maxLines, MaxDocs: maxLines}
	c.Create(&info)
	c.EnsureIndex(sourceIndex)
	c.EnsureIndex(unitIndex)
	return c
}

func (s *Storage) LogsCollections() ([]*storage.Collection, error) {
	var names []struct {
		Name string
	}
	err := s.Apps().Find(nil).All(&names)
	if err != nil {
		return nil, err
	}
	var colls []*storage.Collection
	for _, name := range names {
		colls = append(colls, s.Collection("logs_"+name.Name))
	}
	return colls, nil
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
	return s.Collection("tokens")
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
