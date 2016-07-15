// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package db provides util functions to deal with Gandalf's database.
package db

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2"
)

const (
	DefaultDatabaseURL  = "127.0.0.1:27017"
	DefaultDatabaseName = "gandalf"
)

type Storage struct {
	*storage.Storage
}

// conn reads the gandalf config and calls storage.Open to get a database connection.
//
// Most gandalf packages should probably use this function. storage.Open is intended for
// use when supporting more than one database.
func conn() (*storage.Storage, error) {
	url, dbname := DbConfig()
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

func DbConfig() (string, string) {
	url, _ := config.GetString("database:url")
	if url == "" {
		url = DefaultDatabaseURL
	}
	dbname, _ := config.GetString("database:name")
	if dbname == "" {
		dbname = DefaultDatabaseName
	}
	return url, dbname
}

// Repository returns a reference to the "repository" collection in MongoDB.
func (s *Storage) Repository() *storage.Collection {
	return s.Collection("repository")
}

// User returns a reference to the "user" collection in MongoDB.
func (s *Storage) User() *storage.Collection {
	return s.Collection("user")
}

func (s *Storage) Key() *storage.Collection {
	bodyIndex := mgo.Index{Key: []string{"body"}, Unique: true}
	nameIndex := mgo.Index{Key: []string{"username", "name"}, Unique: true}
	c := s.Collection("key")
	c.EnsureIndex(bodyIndex)
	c.EnsureIndex(nameIndex)
	return c
}
