// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provisioncommon

import (
	"testing"

	"github.com/tsuru/config"
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
		Labels:      map[string]string{"l1": "v1", "l2": "v2"},
		Annotations: map[string]string{"a1": "v1", "a2": "v2"},
	}
	c.Assert(ls.ToLabels(), check.DeepEquals, map[string]string{
		"tsuru.io/l1": "v1",
		"tsuru.io/l2": "v2",
	})
	c.Assert(ls.ToAnnotations(), check.DeepEquals, map[string]string{
		"tsuru.io/a1": "v1",
		"tsuru.io/a2": "v2",
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
			"l2":                "v2",
			"tsuru.io/app-name": "app1",
			"app-name":          "app2",
		},
		Annotations: map[string]string{
			"l1":                "v3",
			"l3":                "v4",
			"tsuru.io/l3":       "v5",
			"l4":                "v6",
			"tsuru.io/app-name": "appan1",
			"app-name":          "appan2",
		},
	}
	c.Assert(ls.getLabel("app-name"), check.Equals, "app1")
	c.Assert(ls.getLabel("l1"), check.Equals, "v1")
	c.Assert(ls.getLabel("l3"), check.Equals, "v5")
	c.Assert(ls.getLabel("l4"), check.Equals, "v6")
}

func (s *S) TestPodLabels(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers")
	a := provisiontest.NewFakeApp("myapp", "cobol", 0)
	ls, err := PodLabels(a, "p1", "myimg", 3)
	c.Assert(err, check.IsNil)
	c.Assert(ls, check.DeepEquals, &LabelSet{
		Labels: map[string]string{
			"is-tsuru":             "true",
			"is-build":             "true",
			"is-stopped":           "false",
			"app-name":             "myapp",
			"app-process":          "p1",
			"app-process-replicas": "3",
			"app-platform":         "cobol",
			"app-pool":             "test-default",
			"router-name":          "fake",
			"router-type":          "fake",
			"provisioner":          "kubernetes",
		},
		Annotations: map[string]string{
			"build-image": "myimg",
		},
	})
	ls, err = PodLabels(a, "p1", "", 3)
	c.Assert(err, check.IsNil)
	c.Assert(ls, check.DeepEquals, &LabelSet{
		Labels: map[string]string{
			"is-tsuru":             "true",
			"is-build":             "false",
			"is-stopped":           "false",
			"app-name":             "myapp",
			"app-process":          "p1",
			"app-process-replicas": "3",
			"app-platform":         "cobol",
			"app-pool":             "test-default",
			"router-name":          "fake",
			"router-type":          "fake",
			"provisioner":          "kubernetes",
		},
		Annotations: map[string]string{
			"build-image": "",
		},
	})
}

func (s *S) TestNodeContainerLabels(c *check.C) {
	c.Assert(NodeContainerLabels("name", "pool", "provisioner", nil), check.DeepEquals, &LabelSet{
		Labels: map[string]string{
			"is-tsuru":            "true",
			"is-node-container":   "true",
			"provisioner":         "provisioner",
			"node-container-name": "name",
			"node-container-pool": "pool",
		},
	})
	c.Assert(NodeContainerLabels("name", "pool", "provisioner", map[string]string{"a": "1"}), check.DeepEquals, &LabelSet{
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
