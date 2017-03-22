// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provisioncommon

import (
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) TestLabelSetConversion(c *check.C) {
	ls := LabelSet{
		Labels: map[string]string{"l1": "v1", "l2": "v2"},
		Prefix: "tsuru.io/",
	}
	c.Assert(ls.ToLabels(), check.DeepEquals, map[string]string{
		"tsuru.io/l1": "v1",
		"tsuru.io/l2": "v2",
	})
}

func (s *S) TestLabelSetSelectors(c *check.C) {
	ls := LabelSet{
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

func (s *S) TestLabelSetGetLabel(c *check.C) {
	ls := LabelSet{
		Labels: map[string]string{
			"l1":                "v1",
			"tsuru.io/app-name": "app1",
			"app-name":          "app2",
		},
		Prefix: "tsuru.io/",
	}
	c.Assert(ls.getLabel("app-name"), check.Equals, "app1")
	c.Assert(ls.getLabel("l1"), check.Equals, "v1")
}

func (s *S) TestServiceLabels(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers")
	a := provisiontest.NewFakeApp("myapp", "cobol", 0)
	opts := ServiceLabelsOpts{
		App:         a,
		Replicas:    3,
		Process:     "p1",
		BuildImage:  "myimg",
		IsBuild:     true,
		Provisioner: "kubernetes",
	}
	ls, err := ServiceLabels(opts)
	c.Assert(err, check.IsNil)
	c.Assert(ls, check.DeepEquals, &LabelSet{
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
			"restarts":             "0",
			"build-image":          "myimg",
		},
	})
}

func (s *S) TestNodeContainerLabels(c *check.C) {
	c.Assert(NodeContainerLabels(NodeContainerLabelsOpts{
		Config: &nodecontainer.NodeContainerConfig{Name: "name"}, Pool: "pool", Provisioner: "provisioner"}), check.DeepEquals, &LabelSet{
		Labels: map[string]string{
			"is-tsuru":            "true",
			"is-node-container":   "true",
			"provisioner":         "provisioner",
			"node-container-name": "name",
			"node-container-pool": "pool",
		},
	})
	c.Assert(NodeContainerLabels(NodeContainerLabelsOpts{
		Config: &nodecontainer.NodeContainerConfig{
			Name:   "name",
			Config: docker.Config{Labels: map[string]string{"a": "1"}},
		}, Pool: "pool", Provisioner: "provisioner"}), check.DeepEquals, &LabelSet{
		Labels: map[string]string{
			"is-tsuru":            "true",
			"is-node-container":   "true",
			"provisioner":         "provisioner",
			"node-container-name": "name",
			"node-container-pool": "pool",
			"a": "1",
		},
	})
}
