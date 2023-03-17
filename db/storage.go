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

// Jobs returns the jobs collection from MongoDB.
func (s *Storage) Jobs() *storage.Collection {
	jobNameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.Collection("jobs")
	c.EnsureIndex(jobNameIndex)
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
	allowedSchemeIndex := mgo.Index{Key: []string{"allowed.scheme"}}
	latestTargetKindIndex := mgo.Index{Key: []string{"target.value", "kind.name", "-starttime"}, Background: true}
	latestTargetIndex := mgo.Index{Key: []string{"target.value", "-starttime"}, Background: true}
	latestExtraTargetIndex := mgo.Index{Key: []string{"extratargets.target.value", "-starttime"}, Background: true}

	c := s.Collection("events")
	c.EnsureIndex(ownerIndex)
	c.EnsureIndex(targetIndex)
	c.EnsureIndex(extraTargetIndex)
	c.EnsureIndex(kindIndex)
	c.EnsureIndex(startTimeIndex)
	c.EnsureIndex(uniqueIdIndex)
	c.EnsureIndex(runningIndex)
	c.EnsureIndex(allowedSchemeIndex)
	c.EnsureIndex(latestTargetKindIndex)
	c.EnsureIndex(latestTargetIndex)
	c.EnsureIndex(latestExtraTargetIndex)
	return c
}

func (s *Storage) EventBlocks() *storage.Collection {
	index := mgo.Index{Key: []string{"ownername", "kindname", "target"}}
	startTimeIndex := mgo.Index{Key: []string{"-starttime"}}
	activeIndex := mgo.Index{Key: []string{"active", "-starttime"}}

	c := s.Collection("event_blocks")
	c.EnsureIndex(index)
	c.EnsureIndex(startTimeIndex)
	c.EnsureIndex(activeIndex)

	return c
}

func (s *Storage) InstallHosts() *storage.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.Collection("install_hosts")
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
