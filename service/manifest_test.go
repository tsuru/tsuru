// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"net/http"

	"github.com/tsuru/tsuru/permission"
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
			Method: http.MethodPost,
			Path:   "rules/{ruleId}/sync",
			Action: "rules.sync",
		}},
	}
	err = svc.IngestManifest(context.TODO(), manifest, false)
	c.Assert(err, check.IsNil)

	exists, err := permission.ExistsDynamic(context.TODO(), "service-action.manifest-ingest-register.rules.sync")
	c.Assert(err, check.IsNil)
	c.Assert(exists, check.Equals, true)

	stored, err := Get(context.TODO(), svc.Name)
	c.Assert(err, check.IsNil)
	c.Assert(stored.Manifest, check.DeepEquals, &ServiceManifest{
		Enabled:       true,
		StrictActions: true,
		Operations: []ManifestOperation{{
			Method: http.MethodPost,
			Path:   "rules/{ruleId}/sync",
			Action: "rules.sync",
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
	exists, err := permission.ExistsDynamic(context.TODO(), "service-action.manifest-ingest-guarded.rules.sync")
	c.Assert(err, check.IsNil)
	c.Assert(exists, check.Equals, true)
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
			Method: http.MethodGet,
			Path:   "/rules",
			Action: "rules.list",
		}},
	}, true)
	c.Assert(err, check.IsNil)

	stored, getErr := Get(context.TODO(), svc.Name)
	c.Assert(getErr, check.IsNil)
	c.Assert(stored.Manifest.Operations[0].Action, check.Equals, "rules.list")
	exists, err := permission.ExistsDynamic(context.TODO(), "service-action.manifest-ingest-force.rules.sync")
	c.Assert(err, check.IsNil)
	c.Assert(exists, check.Equals, false)
	exists, err = permission.ExistsDynamic(context.TODO(), "service-action.manifest-ingest-force.rules.list")
	c.Assert(err, check.IsNil)
	c.Assert(exists, check.Equals, true)
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
			{Method: http.MethodGet, Path: "/rules", Action: "rules.duplicate"},
			{Method: http.MethodPost, Path: "/rules/{id}", Action: "rules.duplicate"},
		},
	}, false)
	c.Assert(err, check.ErrorMatches, `duplicate manifest operation action "rules\.duplicate"`)
}

func (s *S) TestManifestGrantConflicts(c *check.C) {
	err := Create(context.TODO(), Service{
		Name:       "manifest-conflicts",
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
		OwnerTeams: []string{s.team.Name},
		Manifest: &ServiceManifest{
			Enabled: true,
			Operations: []ManifestOperation{{
				Method: http.MethodPost,
				Path:   "/rules/{ruleId}/sync",
				Action: "rules.sync",
			}},
		},
	})
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole(context.TODO(), "manifest-conflicts-role", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddDynamicPermissions(context.TODO(), "service-action.manifest-conflicts.rules.sync")
	c.Assert(err, check.IsNil)

	conflicts, err := manifestGrantConflicts(context.TODO(), "manifest-conflicts", []string{"rules.sync"}, nil)
	c.Assert(err, check.IsNil)
	c.Assert(conflicts, check.DeepEquals, []ManifestGrantConflict{{
		Action: "rules.sync",
		Roles:  []string{"manifest-conflicts-role"},
	}})
}

func (s *S) TestManifestGrantConflictsAncestorGrants(c *check.C) {
	err := Create(context.TODO(), Service{
		Name:       "manifest-ancestor-conflicts",
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
		OwnerTeams: []string{s.team.Name},
		Manifest: &ServiceManifest{
			Enabled: true,
			Operations: []ManifestOperation{{
				Method: http.MethodPost,
				Path:   "/rules/{ruleId}/sync",
				Action: "rules.sync",
			}, {
				Method: http.MethodGet,
				Path:   "/status",
				Action: "status.read",
			}},
		},
	})
	c.Assert(err, check.IsNil)
	prefixRole, err := permission.NewRole(context.TODO(), "manifest-ancestor-prefix-role", "team", "")
	c.Assert(err, check.IsNil)
	err = prefixRole.AddDynamicPermissions(context.TODO(), "service-action.manifest-ancestor-conflicts.rules")
	c.Assert(err, check.IsNil)
	serviceRole, err := permission.NewRole(context.TODO(), "manifest-ancestor-service-role", "team", "")
	c.Assert(err, check.IsNil)
	err = serviceRole.AddDynamicPermissions(context.TODO(), "service-action.manifest-ancestor-conflicts")
	c.Assert(err, check.IsNil)

	nextManifest := &ServiceManifest{
		Enabled: true,
		Operations: []ManifestOperation{{
			Method: http.MethodGet,
			Path:   "/status",
			Action: "status.read",
		}},
	}
	conflicts, err := manifestGrantConflicts(context.TODO(), "manifest-ancestor-conflicts", []string{"rules.sync"}, nextManifest)
	c.Assert(err, check.IsNil)
	c.Assert(conflicts, check.DeepEquals, []ManifestGrantConflict{{
		Action: "rules.sync",
		Roles:  []string{"manifest-ancestor-prefix-role"},
	}})

	emptyManifest := &ServiceManifest{Enabled: true}
	conflicts, err = manifestGrantConflicts(context.TODO(), "manifest-ancestor-conflicts", []string{"rules.sync", "status.read"}, emptyManifest)
	c.Assert(err, check.IsNil)
	c.Assert(conflicts, check.DeepEquals, []ManifestGrantConflict{{
		Action: "rules.sync",
		Roles:  []string{"manifest-ancestor-prefix-role", "manifest-ancestor-service-role"},
	}, {
		Action: "status.read",
		Roles:  []string{"manifest-ancestor-service-role"},
	}})
}

func (s *S) TestManifestGrantConflictsAncestorGrantStillCoversRemainingAction(c *check.C) {
	err := Create(context.TODO(), Service{
		Name:       "manifest-ancestor-kept",
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
		OwnerTeams: []string{s.team.Name},
		Manifest: &ServiceManifest{
			Enabled: true,
			Operations: []ManifestOperation{{
				Method: http.MethodPost,
				Path:   "/rules/{ruleId}/sync",
				Action: "rules.sync",
			}, {
				Method: http.MethodGet,
				Path:   "/rules",
				Action: "rules.list",
			}},
		},
	})
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole(context.TODO(), "manifest-ancestor-kept-role", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddDynamicPermissions(context.TODO(), "service-action.manifest-ancestor-kept.rules")
	c.Assert(err, check.IsNil)

	nextManifest := &ServiceManifest{
		Enabled: true,
		Operations: []ManifestOperation{{
			Method: http.MethodGet,
			Path:   "/rules",
			Action: "rules.list",
		}},
	}
	conflicts, err := manifestGrantConflicts(context.TODO(), "manifest-ancestor-kept", []string{"rules.sync"}, nextManifest)
	c.Assert(err, check.IsNil)
	c.Assert(conflicts, check.HasLen, 0)
}
