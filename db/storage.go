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
	"fmt"

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

func LogConn() (*LogStorage, error) {
	var (
		strg LogStorage
		err  error
	)
	url, dbname := DbConfig("logdb-")
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

// Services returns the services collection from MongoDB.
func (s *Storage) Services() *storage.Collection {
	return s.Collection("services")
}

// ServiceInstances returns the services_instances collection from MongoDB.
func (s *Storage) ServiceInstances() *storage.Collection {
	return s.Collection("service_instances")
}

// Pools returns the pool collection.
func (s *Storage) Pools() *storage.Collection {
	return s.Collection("pool")
}

// PoolsConstraints return the pool constraints collection.
func (s *Storage) PoolsConstraints() *storage.Collection {
	poolConstraintIndex := mgo.Index{Key: []string{"poolexpr", "field"}, Unique: true}
	c := s.Collection("pool_constraints")
	c.EnsureIndex(poolConstraintIndex)
	return c
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

// SAMLRequests returns the saml_requests from MongoDB.
func (s *Storage) SAMLRequests() *storage.Collection {
	id := mgo.Index{Key: []string{"id"}}
	coll := s.Collection("saml_requests")
	coll.EnsureIndex(id)
	return coll
}

var logCappedInfo = mgo.CollectionInfo{
	Capped:   true,
	MaxBytes: 200 * 5000,
	MaxDocs:  5000,
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

func (s *Storage) Roles() *storage.Collection {
	return s.Collection("roles")
}

func (s *Storage) Limiter() *storage.Collection {
	return s.Collection("limiter")
}

func (s *Storage) Events() *storage.Collection {
	ownerIndex := mgo.Index{Key: []string{"owner.name"}}
	targetIndex := mgo.Index{Key: []string{"target.value"}}
	extraTargetIndex := mgo.Index{Key: []string{"extratargets.target.value"}}
	kindIndex := mgo.Index{Key: []string{"kind.name"}}
	startTimeIndex := mgo.Index{Key: []string{"-starttime"}}
	uniqueIdIndex := mgo.Index{Key: []string{"uniqueid"}}
	runningIndex := mgo.Index{Key: []string{"running"}}
	c := s.Collection("events")
	c.EnsureIndex(ownerIndex)
	c.EnsureIndex(targetIndex)
	c.EnsureIndex(extraTargetIndex)
	c.EnsureIndex(kindIndex)
	c.EnsureIndex(startTimeIndex)
	c.EnsureIndex(uniqueIdIndex)
	c.EnsureIndex(runningIndex)
	return c
}

func (s *Storage) EventBlocks() *storage.Collection {
	index := mgo.Index{Key: []string{"ownername", "kindname", "target"}}
	startTimeIndex := mgo.Index{Key: []string{"-starttime"}}
	c := s.Collection("event_blocks")
	c.EnsureIndex(index)
	c.EnsureIndex(startTimeIndex)
	return c
}

func (s *Storage) InstallHosts() *storage.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.Collection("install_hosts")
	c.EnsureIndex(nameIndex)
	return c
}

func (s *Storage) Volumes() *storage.Collection {
	c := s.Collection("volumes")
	return c
}

func (s *Storage) VolumeBinds() *storage.Collection {
	c := s.Collection("volume_binds")
	return c
}
