// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"time"
)

var (
	ErrInvalidUnitStatus = errors.New("invalid status")
)

// Status represents the status of a unit in tsuru.
type UnitStatus string

func (s UnitStatus) String() string {
	return string(s)
}

// Unit represents a provision unit. Can be a machine, container or anything
// IP-addressable.
type Unit struct {
	ID           string
	Name         string
	AppName      string
	ProcessName  string
	Type         string
	IP           string
	InternalIP   string
	Status       UnitStatus
	StatusReason string
	Address      *url.URL  // TODO: deprecate after drop docker provisioner
	Addresses    []url.URL // TODO: deprecate after drop docker provisioner
	Version      int
	Routable     bool
	Restarts     *int32
	CreatedAt    *time.Time
	Ready        *bool
}

// GetName returns the name of the unit.
func (u *Unit) GetID() string {
	return u.ID
}

// GetIp returns the Unit.IP.
func (u *Unit) GetIp() string {
	return u.IP
}

func (u *Unit) MarshalJSON() ([]byte, error) {
	type UnitForMarshal Unit
	var host, port string
	if u.Address != nil {
		host, port, _ = net.SplitHostPort(u.Address.Host)
	}
	// New fields added for compatibility with old routes returning containers.
	return json.Marshal(&struct {
		*UnitForMarshal
		HostAddr string
		HostPort string
		IP       string
	}{
		UnitForMarshal: (*UnitForMarshal)(u),
		HostAddr:       host,
		HostPort:       port,
		IP:             u.IP,
	})
}

// Available returns true if the unit is available. It will return true
// whenever the unit itself is available, even when the application process is
// not.
func (u *Unit) Available() bool {
	return u.Status == UnitStatusStarted ||
		u.Status == UnitStatusStarting ||
		u.Status == UnitStatusError
}

func ParseUnitStatus(status string) (UnitStatus, error) {
	switch status {
	case "created":
		return UnitStatusCreated, nil
	case "building":
		return UnitStatusBuilding, nil
	case "error":
		return UnitStatusError, nil
	case "started":
		return UnitStatusStarted, nil
	case "starting":
		return UnitStatusStarting, nil
	case "stopped":
		return UnitStatusStopped, nil
	case "success":
		return UnitStatusSucceeded, nil
	}
	return UnitStatus(""), ErrInvalidUnitStatus
}

// Flow:

// +---------+             +----------+                 +---------+
// | Created | +---------> | Starting | +-------------> | Started |
// +---------+             +----------+                 +---------+
//
//	^                         + +
//	|                         | |
//	|                         | |
//	|                         | |
//	v                         | |
//	+-------+                 | |
//	| Error | +---------------+ |
//	+-------+ ------------------+
const (
	// UnitStatusCreated is the initial status of a unit in the database,
	// it should transition shortly to a more specific status
	UnitStatusCreated = UnitStatus("created")

	// UnitStatusBuilding is the status for units being provisioned by the
	// provisioner, like in the deployment.
	UnitStatusBuilding = UnitStatus("building")

	// UnitStatusError is the status for units that failed to start, because of
	// an application error.
	UnitStatusError = UnitStatus("error")

	// UnitStatusStarting is set when the container is started in docker.
	UnitStatusStarting = UnitStatus("starting")

	// StatusStarted is for cases where the unit is up and running, and bound
	// to the proper status, it's set by RegisterUnit and SetUnitStatus.
	UnitStatusStarted = UnitStatus("started")

	// UnitStatusStopped is for cases where the unit has been stopped.
	UnitStatusStopped = UnitStatus("stopped")

	// UnitStatusSucceeded is for alternete cases where the unit has been
	// stopped with succeeded end.
	UnitStatusSucceeded = UnitStatus("succeeded")
)

// UnitMetric represents a a related metrics for an unit.
type UnitMetric struct {
	ID     string
	CPU    string
	Memory string
}
