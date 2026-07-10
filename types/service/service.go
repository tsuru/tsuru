// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"time"
)

type Service struct {
	Manifest *ServiceManifest `bson:"manifest,omitempty" json:"manifest,omitempty"`
}

type ServiceManifest struct {
	Enabled         bool                `bson:"enabled" json:"enabled"`
	StrictActions   bool                `bson:"strict_actions" json:"strict_actions"`
	LegacyCompat    bool                `bson:"legacy_compat" json:"legacy_compat"`
	LegacyEnabledAt *time.Time          `bson:"legacy_enabled_at,omitempty" json:"legacy_enabled_at,omitempty"`
	Operations      []ManifestOperation `bson:"operations" json:"operations"`
}

type ManifestOperation struct {
	Method string `bson:"method" json:"method"`
	Path   string `bson:"path" json:"path"`
	Action string `bson:"action" json:"action"`
}
