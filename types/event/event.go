// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"errors"
	"fmt"
	"time"

	"github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/tracker"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EventData struct {
	ID              primitive.ObjectID `json:"-" bson:"_id"`
	UniqueID        primitive.ObjectID
	Lock            *Target `bson:"lock,omitempty"`
	StartTime       time.Time
	EndTime         time.Time          `bson:",omitempty"`
	ExpireAt        time.Time          `bson:",omitempty"`
	Target          Target             `bson:",omitempty"`
	ExtraTargets    []ExtraTarget      `bson:",omitempty"`
	StartCustomData mongoBSON.RawValue `json:"-" bson:",omitempty"`
	EndCustomData   mongoBSON.RawValue `json:"-" bson:",omitempty"`
	OtherCustomData mongoBSON.RawValue `json:"-" bson:",omitempty"`
	Kind            Kind
	Owner           Owner
	SourceIP        string
	LockUpdateTime  time.Time
	Error           string
	Log             string     `bson:",omitempty"`
	StructuredLog   []LogEntry `bson:",omitempty"`
	CancelInfo      CancelInfo
	Cancelable      bool
	Running         bool
	Allowed         AllowedPermission
	AllowedCancel   AllowedPermission
	Instance        tracker.TrackedInstance
}

type EventInfo struct {
	EventData

	// StartCustomData, EndCustomData and OtherCustomData are legacy fields that will be deprecated on the future
	// just use for compatibility reasons
	StartCustomData LegacyBSONRaw `bson:",omitempty"`
	EndCustomData   LegacyBSONRaw `bson:",omitempty"`
	OtherCustomData LegacyBSONRaw `bson:",omitempty"`

	// CustomData is the new way to access eventData.{StartCustomData, EndCustomData, OtherCustomData}
	// the major advantage is that you can access the data without converting from bson.RawValue
	CustomData EventInfoCustomData
}

type EventInfoCustomData struct {
	Start any
	End   any
	Other any
}

type LegacyBSONRaw struct {
	Kind byte
	Data []byte
}

type Target struct {
	Type  TargetType
	Value string
}

func (id Target) IsValid() bool {
	return id.Type != ""
}

func (id Target) String() string {
	return fmt.Sprintf("%s(%s)", id.Type, id.Value)
}

type ExtraTarget struct {
	Target Target
	Lock   bool
}

type Kind struct {
	Type KindType
	Name string
}

func (k Kind) String() string {
	return k.Name
}

type Owner struct {
	Type OwnerType
	Name string
}

type LogEntry struct {
	Date    time.Time
	Message string
}

func (o Owner) String() string {
	return fmt.Sprintf("%s %s", o.Type, o.Name)
}

type CancelInfo struct {
	Owner     string
	StartTime time.Time
	AckTime   time.Time
	Reason    string
	Asked     bool
	Canceled  bool
}

type AllowedPermission struct {
	Scheme   string
	Contexts []permission.PermissionContext `bson:",omitempty"`
}

type OwnerType string

type KindType string

type TargetType string

var (
	OwnerTypeUser     = OwnerType("user")
	OwnerTypeApp      = OwnerType("app")
	OwnerTypeInternal = OwnerType("internal")
	OwnerTypeToken    = OwnerType("token")

	KindTypePermission = KindType("permission")
	KindTypeInternal   = KindType("internal")

	TargetTypeGlobal          = TargetType("global")
	TargetTypeApp             = TargetType("app")
	TargetTypeJob             = TargetType("job")
	TargetTypeNode            = TargetType("node")
	TargetTypeContainer       = TargetType("container")
	TargetTypePool            = TargetType("pool")
	TargetTypeService         = TargetType("service")
	TargetTypeServiceInstance = TargetType("service-instance")
	TargetTypeTeam            = TargetType("team")
	TargetTypeUser            = TargetType("user")
	TargetTypeRole            = TargetType("role")
	TargetTypePlatform        = TargetType("platform")
	TargetTypePlan            = TargetType("plan")
	TargetTypeNodeContainer   = TargetType("node-container")
	TargetTypeInstallHost     = TargetType("install-host")
	TargetTypeEventBlock      = TargetType("event-block")
	TargetTypeCluster         = TargetType("cluster")
	TargetTypeVolume          = TargetType("volume")
	TargetTypeWebhook         = TargetType("webhook")
	TargetTypeGC              = TargetType("gc")
	TargetTypeRouter          = TargetType("router")

	ErrInvalidTargetType = errors.New("invalid event target type")
)

func GetTargetType(t string) (TargetType, error) {
	switch t {
	case "global":
		return TargetTypeGlobal, nil
	case "app":
		return TargetTypeApp, nil
	case "node":
		return TargetTypeNode, nil
	case "container":
		return TargetTypeContainer, nil
	case "pool":
		return TargetTypePool, nil
	case "service":
		return TargetTypeService, nil
	case "service-instance":
		return TargetTypeServiceInstance, nil
	case "team":
		return TargetTypeTeam, nil
	case "user":
		return TargetTypeUser, nil
	case "role":
		return TargetTypeRole, nil
	case "platform":
		return TargetTypePlatform, nil
	case "plan":
		return TargetTypePlan, nil
	case "node-container":
		return TargetTypeNodeContainer, nil
	case "install-host":
		return TargetTypeInstallHost, nil
	case "event-block":
		return TargetTypeEventBlock, nil
	case "cluster":
		return TargetTypeCluster, nil
	case "volume":
		return TargetTypeVolume, nil
	case "webhook":
		return TargetTypeWebhook, nil
	case "router":
		return TargetTypeRouter, nil
	}
	return TargetType(""), ErrInvalidTargetType
}
