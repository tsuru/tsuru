// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"net/http"

	"github.com/tsuru/tsuru/permission"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestIngestManifestRegistersAndPersists(c *check.C) {
	svc := Service{
		Name:       "manifest-ingest-register",
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
		OwnerTeams: []string{s.team.Name},
	}
	err := Create(context.TODO(), svc)
	c.Assert(err, check.IsNil)
	manifest := &ServiceManifest{
		Enabled:       true,
		StrictActions: true,
		Operations: []ManifestOperation{{
			Name:   "sync-rule",
			Method: "post",
			Path:   "rules/{ruleId}/sync",
			Action: "rules.sync",
		}},
	}
	err = svc.IngestManifest(context.TODO(), manifest, false)
	c.Assert(err, check.IsNil)

	scheme, ok := permission.LookupDynamic("service-action.manifest-ingest-register.rules.sync")
	c.Assert(ok, check.Equals, true)
	c.Assert(scheme.FullName(), check.Equals, "service-action.manifest-ingest-register.rules.sync")

	stored, err := Get(context.TODO(), svc.Name)
	c.Assert(err, check.IsNil)
	c.Assert(stored.Manifest, check.DeepEquals, &ServiceManifest{
		Enabled:       true,
		StrictActions: true,
		Operations: []ManifestOperation{{
			Name:   "sync-rule",
			Method: http.MethodPost,
			Path:   "/rules/{ruleId}/sync",
			Action: "rules.sync",
			Scope:  "entity",
		}},
	})
}

func (s *S) TestIngestManifestRejectsOrphanedDynamicGrants(c *check.C) {
	svc := Service{
		Name:       "manifest-ingest-guarded",
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
		OwnerTeams: []string{s.team.Name},
	}
	err := Create(context.TODO(), svc)
	c.Assert(err, check.IsNil)
	err = svc.IngestManifest(context.TODO(), &ServiceManifest{
		Enabled:       true,
		StrictActions: true,
		Operations: []ManifestOperation{{
			Name:   "sync-rule",
			Method: http.MethodPost,
			Path:   "/rules/{ruleId}/sync",
			Action: "rules.sync",
		}},
	}, false)
	c.Assert(err, check.IsNil)

	role, err := permission.NewRole(context.TODO(), "manifest-guarded-role", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddDynamicPermissions(context.TODO(), "service-action.manifest-ingest-guarded.rules.sync")
	c.Assert(err, check.IsNil)

	err = svc.IngestManifest(context.TODO(), &ServiceManifest{
		Enabled:       true,
		StrictActions: true,
		Operations: []ManifestOperation{{
			Name:   "list-rules",
			Method: http.MethodGet,
			Path:   "/rules",
			Action: "rules.list",
		}},
	}, false)
	c.Assert(err, check.FitsTypeOf, &ManifestConflictError{})
	conflictErr := err.(*ManifestConflictError)
	c.Assert(conflictErr.Conflicts, check.DeepEquals, []ManifestGrantConflict{{
		Action: "rules.sync",
		Roles:  []string{"manifest-guarded-role"},
	}})

	stored, getErr := Get(context.TODO(), svc.Name)
	c.Assert(getErr, check.IsNil)
	c.Assert(stored.Manifest.Operations[0].Action, check.Equals, "rules.sync")
	_, ok := permission.LookupDynamic("service-action.manifest-ingest-guarded.rules.sync")
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestIngestManifestForceRemovesOrphanedDynamicGrants(c *check.C) {
	svc := Service{
		Name:       "manifest-ingest-force",
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
		OwnerTeams: []string{s.team.Name},
	}
	err := Create(context.TODO(), svc)
	c.Assert(err, check.IsNil)
	err = svc.IngestManifest(context.TODO(), &ServiceManifest{
		Enabled:       true,
		StrictActions: true,
		Operations: []ManifestOperation{{
			Name:   "sync-rule",
			Method: http.MethodPost,
			Path:   "/rules/{ruleId}/sync",
			Action: "rules.sync",
		}},
	}, false)
	c.Assert(err, check.IsNil)

	role, err := permission.NewRole(context.TODO(), "manifest-force-role", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddDynamicPermissions(context.TODO(), "service-action.manifest-ingest-force.rules.sync")
	c.Assert(err, check.IsNil)

	err = svc.IngestManifest(context.TODO(), &ServiceManifest{
		Enabled:       true,
		StrictActions: true,
		Operations: []ManifestOperation{{
			Name:   "list-rules",
			Method: http.MethodGet,
			Path:   "/rules",
			Action: "rules.list",
		}},
	}, true)
	c.Assert(err, check.IsNil)

	stored, getErr := Get(context.TODO(), svc.Name)
	c.Assert(getErr, check.IsNil)
	c.Assert(stored.Manifest.Operations[0].Action, check.Equals, "rules.list")
	_, ok := permission.LookupDynamic("service-action.manifest-ingest-force.rules.sync")
	c.Assert(ok, check.Equals, false)
	_, ok = permission.LookupDynamic("service-action.manifest-ingest-force.rules.list")
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestIngestManifestValidation(c *check.C) {
	svc := Service{
		Name:       "manifest-ingest-validate",
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
		OwnerTeams: []string{s.team.Name},
	}
	err := Create(context.TODO(), svc)
	c.Assert(err, check.IsNil)
	err = svc.IngestManifest(context.TODO(), &ServiceManifest{
		Enabled:       true,
		StrictActions: true,
		Operations: []ManifestOperation{
			{Name: "dup", Method: http.MethodGet, Path: "/rules", Action: "rules.list"},
			{Name: "dup", Method: http.MethodPost, Path: "/rules", Action: "rules.sync"},
		},
	}, false)
	c.Assert(err, check.ErrorMatches, `duplicate manifest operation name "dup"`)
}

func (s *S) TestManifestGrantConflicts(c *check.C) {
	err := permission.RegisterDynamic("service-action.manifest-conflicts.rules.sync", []permTypes.ContextType{permTypes.CtxTeam})
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole(context.TODO(), "manifest-conflicts-role", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddDynamicPermissions(context.TODO(), "service-action.manifest-conflicts.rules.sync")
	c.Assert(err, check.IsNil)

	conflicts, err := manifestGrantConflicts(context.TODO(), "manifest-conflicts", []string{"rules.sync"})
	c.Assert(err, check.IsNil)
	c.Assert(conflicts, check.DeepEquals, []ManifestGrantConflict{{
		Action: "rules.sync",
		Roles:  []string{"manifest-conflicts-role"},
	}})
}
