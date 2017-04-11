// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"time"

	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2/bson"
)

type Container struct {
	MongoID                 bson.ObjectId `bson:"_id,omitempty"`
	ID                      string
	AppName                 string
	ProcessName             string
	Type                    string
	IP                      string
	HostAddr                string
	HostPort                string
	PrivateKey              string
	Status                  string
	StatusBeforeError       string
	Version                 string
	Image                   string
	Name                    string
	User                    string
	BuildingImage           string
	LastStatusUpdate        time.Time
	LastSuccessStatusUpdate time.Time
	LockedUntil             time.Time
	Routable                bool `bson:"-"`
	ExposedPort             string
}

type DockerLogConfig struct {
	Driver  string
	LogOpts map[string]string
}

type HealingEvent struct {
	ID               interface{} `bson:"_id"`
	StartTime        time.Time
	EndTime          time.Time
	Action           string
	Reason           string
	Extra            interface{}
	FailingNode      provision.NodeSpec
	CreatedNode      provision.NodeSpec
	FailingContainer Container
	CreatedContainer Container
	Successful       bool
	Error            string
}
