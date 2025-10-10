// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision_test

import (
	"context"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	check "gopkg.in/check.v1"
)

func (s *S) TestLabelSetConversion(c *check.C) {
	ls := provision.LabelSet{
		Labels: map[string]string{"l1": "v1", "l2": "v2"},
		Prefix: "tsuru.io/",
	}
	c.Assert(ls.ToLabels(), check.DeepEquals, map[string]string{
		"tsuru.io/l1": "v1",
		"tsuru.io/l2": "v2",
	})
}

func (s *S) TestLabelSetSelectors(c *check.C) {
	ls := provision.LabelSet{
		Labels: map[string]string{
			"l1":          "v1",
			"l2":          "v2",
			"app-name":    "app",
			"app-process": "proc",
			"is-build":    "false",
		},
		Prefix: "tsuru.io/",
	}
	c.Assert(ls.ToBaseSelector(), check.DeepEquals, map[string]string{
		"tsuru.io/app-name":    "app",
		"tsuru.io/app-process": "proc",
		"tsuru.io/is-build":    "false",
	})
	c.Assert(ls.ToAppSelector(), check.DeepEquals, map[string]string{
		"tsuru.io/app-name": "app",
	})
}

func (s *S) TestProcessLabels(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers")
	a := provisiontest.NewFakeApp("myapp", "cobol", 0)
	a.Tags = []string{
		"tag1=1",
		"tag2",
		"space tag",
		"weird %$! tag",
		"tag3=a=b=c obla di obla da",
	}
	opts := provision.ProcessLabelsOpts{
		App:     a,
		Process: "p1",
	}
	ls, err := provision.ProcessLabels(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	c.Assert(ls, check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"is-tsuru":        "true",
			"is-stopped":      "false",
			"app-name":        "myapp",
			"app-process":     "p1",
			"app-platform":    "cobol",
			"app-pool":        "test-default",
			"app-team":        "",
			"custom-tag-tag1": "1",
			"custom-tag-tag2": "",
			"custom-tag-tag3": "a=b=c obla di obla da",
		},
	})
}

func (s *S) TestServiceLabels(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers")
	a := provisiontest.NewFakeApp("myapp", "cobol", 0)
	opts := provision.ServiceLabelsOpts{
		App:     a,
		Process: "p1",
		Version: 9,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsBuild: true,
		},
	}
	ls, err := provision.ServiceLabels(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	c.Assert(ls, check.DeepEquals, &provision.LabelSet{
		RawLabels: map[string]string{
			"app":                          "myapp-p1",
			"app.kubernetes.io/component":  "tsuru-app",
			"app.kubernetes.io/instance":   "myapp-p1",
			"app.kubernetes.io/managed-by": "tsuru",
			"app.kubernetes.io/name":       "myapp",
			"app.kubernetes.io/version":    "v9",
			"version":                      "v9",
		},
		Labels: map[string]string{
			"is-tsuru":        "true",
			"is-build":        "true",
			"is-service":      "true",
			"is-stopped":      "false",
			"is-isolated-run": "false",
			"app-name":        "myapp",
			"app-team":        "",
			"app-process":     "p1",
			"app-platform":    "cobol",
			"app-pool":        "test-default",
			"app-version":     "9",
		},
	})
}

func (s *S) TestNodeLabels(c *check.C) {
	opts := provision.NodeLabelsOpts{
		Addr:         "localhost:80",
		Pool:         "mypool",
		CustomLabels: map[string]string{"data": "1"},
		Prefix:       "myprefix",
	}
	c.Assert(provision.NodeLabels(opts), check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"internal-node-addr": "localhost:80",
			"pool":               "mypool",
			"data":               "1",
		},
		Prefix: "myprefix",
	})
}

func (s *S) TestLabelSet_Merge(c *check.C) {
	src := &provision.LabelSet{
		Labels:    map[string]string{"l0": "w0", "l1": "w1"},
		RawLabels: map[string]string{"l2": "w2"},
		Prefix:    "myprefix.example.com/",
	}
	override := &provision.LabelSet{
		Labels:    map[string]string{"l1": "v1"},
		RawLabels: map[string]string{"l2": "v2", "l3": "v3"},
	}
	ls := src.Merge(override)
	expected := &provision.LabelSet{
		Labels:    map[string]string{"l0": "w0", "l1": "v1"},
		RawLabels: map[string]string{"l2": "v2", "l3": "v3"},
		Prefix:    "myprefix.example.com/",
	}
	c.Assert(ls, check.DeepEquals, expected)
}

func (s *S) TestPDBLabels(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "haskell", 42)
	app.TeamOwner = "team-one"
	ls := provision.PDBLabels(provision.PDBLabelsOpts{
		App:     app,
		Prefix:  "tsuru.io/",
		Process: "web",
	})
	c.Assert(ls, check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"is-tsuru":    "true",
			"app-name":    "myapp",
			"app-process": "web",
			"app-team":    "team-one",
		},
		Prefix: "tsuru.io/",
	})
}

func (s *S) TestJobLabels(c *check.C) {
	job := provisiontest.NewFakeJob("my-job", "test-pool", "test-team")
	job.Spec.Manual = true
	ls := provision.JobLabels(context.TODO(), job)

	c.Assert(ls, check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"is-tsuru":   "true",
			"job-name":   "my-job",
			"job-team":   "test-team",
			"job-pool":   "test-pool",
			"is-job":     "true",
			"job-manual": "true",
			"is-service": "true",
			"is-build":   "false",
		},
		RawLabels: map[string]string{
			"app.kubernetes.io/name":       "tsuru-job",
			"app.kubernetes.io/instance":   "my-job",
			"app.kubernetes.io/component":  "job",
			"app.kubernetes.io/managed-by": "tsuru",
		},
		Prefix: "tsuru.io/",
	})
}

func (s *S) TestJobLabelsWithTags(c *check.C) {
	job := provisiontest.NewFakeJob("my-job", "test-pool", "test-team")
	job.Tags = []string{
		"tag1=1",
		"tag2",
		"space tag",
		"weird %$! tag",
		"tag3=a=b=c obla di obla da",
	}
	ls := provision.JobLabels(context.TODO(), job)

	c.Assert(ls, check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"is-tsuru":        "true",
			"job-name":        "my-job",
			"job-team":        "test-team",
			"job-pool":        "test-pool",
			"is-job":          "true",
			"job-manual":      "false",
			"is-service":      "true",
			"is-build":        "false",
			"custom-tag-tag1": "1",
			"custom-tag-tag2": "",
			"custom-tag-tag3": "a=b=c obla di obla da",
		},
		RawLabels: map[string]string{
			"app.kubernetes.io/name":       "tsuru-job",
			"app.kubernetes.io/instance":   "my-job",
			"app.kubernetes.io/component":  "job",
			"app.kubernetes.io/managed-by": "tsuru",
		},
		Prefix: "tsuru.io/",
	})
}
