// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision_test

import (
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
	c.Assert(ls.ToSelector(), check.DeepEquals, map[string]string{
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
	opts := provision.ProcessLabelsOpts{
		App:         a,
		Process:     "p1",
		Provisioner: "kubernetes",
	}
	ls, err := provision.ProcessLabels(opts)
	c.Assert(err, check.IsNil)
	c.Assert(ls, check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"is-tsuru":     "true",
			"is-stopped":   "false",
			"is-deploy":    "false",
			"app-name":     "myapp",
			"app-process":  "p1",
			"app-platform": "cobol",
			"app-pool":     "test-default",
			"router-name":  "fake",
			"router-type":  "fake",
			"provisioner":  "kubernetes",
			"builder":      "",
		},
	})
}

func (s *S) TestServiceLabels(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers")
	a := provisiontest.NewFakeApp("myapp", "cobol", 0)
	opts := provision.ServiceLabelsOpts{
		App:      a,
		Replicas: 3,
		Process:  "p1",
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			BuildImage:  "myimg",
			IsBuild:     true,
			Provisioner: "kubernetes",
			Builder:     "docker",
		},
	}
	ls, err := provision.ServiceLabels(opts)
	c.Assert(err, check.IsNil)
	c.Assert(ls, check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"is-tsuru":             "true",
			"is-build":             "true",
			"is-service":           "true",
			"is-stopped":           "false",
			"is-isolated-run":      "false",
			"is-deploy":            "false",
			"app-name":             "myapp",
			"app-process":          "p1",
			"app-process-replicas": "3",
			"app-platform":         "cobol",
			"app-pool":             "test-default",
			"router-name":          "fake",
			"router-type":          "fake",
			"provisioner":          "kubernetes",
			"build-image":          "myimg",
			"builder":              "docker",
		},
	})
}

func (s *S) TestNodeContainerLabels(c *check.C) {
	opts := provision.NodeContainerLabelsOpts{Name: "name", Pool: "pool", Provisioner: "provisioner"}
	c.Assert(provision.NodeContainerLabels(opts), check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"is-tsuru":            "true",
			"is-node-container":   "true",
			"provisioner":         "provisioner",
			"node-container-name": "name",
			"node-container-pool": "pool",
		},
	})
	opts.CustomLabels = map[string]string{"a": "1"}
	c.Assert(provision.NodeContainerLabels(opts), check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"is-tsuru":            "true",
			"is-node-container":   "true",
			"provisioner":         "provisioner",
			"node-container-name": "name",
			"node-container-pool": "pool",
			"a":                   "1",
		},
	})
}

func (s *S) TestNodeLabels(c *check.C) {
	opts := provision.NodeLabelsOpts{
		IaaSID:       "vm-1234",
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
			"iaas-id":            "vm-1234",
		},
		Prefix: "myprefix",
	})
}

func (s *S) TestLabelSet_WithoutAppReplicas(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers")
	a := provisiontest.NewFakeApp("myapp", "cobol", 0)
	opts := provision.ServiceLabelsOpts{
		App:      a,
		Replicas: 3,
		Process:  "p1",
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			BuildImage:  "myimg",
			IsBuild:     true,
			Provisioner: "kubernetes",
			Builder:     "docker",
		},
	}
	ls, err := provision.ServiceLabels(opts)
	c.Assert(err, check.IsNil)
	c.Assert(ls.Labels["app-process-replicas"], check.Equals, "3")
	c.Assert(ls.WithoutAppReplicas().Labels["app-process-replicas"], check.Equals, "")
}
