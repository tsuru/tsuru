// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package db encapsulates tsuru connection with MongoDB.
//
// The function Open dials to MongoDB and returns a connection (represented by
// the storage.Storage type). It manages an internal pool of connections, and
// reconnects in case of failures. That means that you should not store
// references to the connection, but always call Open.
package db

import (
	"github.com/globocom/go-mgo"
	"github.com/globocom/config"
	"labix.org/v2/mgo"
)

const (
	DefaultDatabaseURL  = "127.0.0.1:27017"
	DefaultDatabaseName = "tsuru"
)

type tsrStorage storage.Storage

// Conn reads the tsuru config and calls Open to get a database connection.
//
// Most tsuru packages should probably use this function. Open is intended for
// use when supporting more than one database.
func Conn() (*storage.Storage, error) {
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

// Apps returns the apps collection from MongoDB.
func (s *tsrStorage) Apps() *storage.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
    var strg interface{}
    strg = s
	c := strg.(*storage.Storage).Collection("apps")
	c.EnsureIndex(nameIndex)
	return c
}

func (s *tsrStorage) Deploys() *storage.Collection {
    var strg interface{}
    strg = s
	return strg.(*storage.Storage).Collection("deploys")
}

// Platforms returns the platforms collection from MongoDB.
func (s *tsrStorage) Platforms() *storage.Collection {
    var strg interface{}
    strg = s
	return strg.(*storage.Storage).Collection("platforms")
}

// Logs returns the logs collection from MongoDB.
func (s *tsrStorage) Logs() *storage.Collection {
	appNameIndex := mgo.Index{Key: []string{"appname"}}
	sourceIndex := mgo.Index{Key: []string{"source"}}
	dateAscIndex := mgo.Index{Key: []string{"date"}}
	dateDescIndex := mgo.Index{Key: []string{"-date"}}
    var strg interface{}
    strg = s
	c := strg.(*storage.Storage).Collection("logs")
	c.EnsureIndex(appNameIndex)
	c.EnsureIndex(sourceIndex)
	c.EnsureIndex(dateAscIndex)
	c.EnsureIndex(dateDescIndex)
	return c
}

// Services returns the services collection from MongoDB.
func (s *tsrStorage) Services() *storage.Collection {
    var strg interface{}
    strg = s
	return strg.(*storage.Storage).Collection("services")
}

// ServiceInstances returns the services_instances collection from MongoDB.
func (s *tsrStorage) ServiceInstances() *storage.Collection {
    var strg interface{}
    strg = s
	return strg.(*storage.Storage).Collection("service_instances")
}

// Users returns the users collection from MongoDB.
func (s *tsrStorage) Users() *storage.Collection {
	emailIndex := mgo.Index{Key: []string{"email"}, Unique: true}
    var strg interface{}
    strg = s
    c := strg.(*storage.Storage).Collection("users")
	c.EnsureIndex(emailIndex)
	return c
}

func (s *tsrStorage) Tokens() *storage.Collection {
    var strg interface{}
    strg = s
	return strg.(*storage.Storage).Collection("tokens")
}

func (s *tsrStorage) PasswordTokens() *storage.Collection {
    var strg interface{}
    strg = s
	return strg.(*storage.Storage).Collection("password_tokens")
}

func (s *tsrStorage) UserActions() *storage.Collection {
    var strg interface{}
    strg = s
	return strg.(*storage.Storage).Collection("user_actions")
}

// Teams returns the teams collection from MongoDB.
func (s *tsrStorage) Teams() *storage.Collection {
    var strg interface{}
    strg = s
	return strg.(*storage.Storage).Collection("teams")
}

// Quota returns the quota collection from MongoDB.
func (s *tsrStorage) Quota() *storage.Collection {
	userIndex := mgo.Index{Key: []string{"owner"}, Unique: true}
    var strg interface{}
    strg = s
	c := strg.(*storage.Storage).Collection("quota")
	c.EnsureIndex(userIndex)
	return c
}
