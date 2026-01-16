// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracker

import (
	"context"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types/tracker"
	check "gopkg.in/check.v1"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "api_tracker_pkg_tests")
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *S) SetUpTest(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *S) Test_InstanceService(c *check.C) {
	svc, err := InstanceService()
	c.Assert(err, check.IsNil)
	svc.(*instanceTracker).Shutdown(context.Background())
	instances, err := svc.LiveInstances(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.HasLen, 1)
	c.Assert(instances[0].Name, check.Not(check.Equals), "")
	c.Assert(len(instances[0].Addresses) > 0, check.Equals, true)
}

func (s *S) Test_InstanceService_CurrentInstance(c *check.C) {
	svc, err := InstanceService()
	c.Assert(err, check.IsNil)
	svc.(*instanceTracker).Shutdown(context.Background())
	instance, err := svc.CurrentInstance(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Not(check.Equals), "")
	c.Assert(len(instance.Addresses) > 0, check.Equals, true)
}

func (s *S) Test_K8sInstanceService(c *check.C) {
	cli := fake.NewSimpleClientset()
	svc := &k8sInstanceTracker{
		cli:     cli,
		ns:      "tsuru-system",
		service: "tsuru-api",
		currentInstance: tracker.TrackedInstance{
			Name:      "tsuru-api-0",
			Addresses: []string{"10.9.9.9"},
		},
	}

	cli.DiscoveryV1().EndpointSlices(svc.ns).Create(context.TODO(), &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: svc.service + "-slice",
			Labels: map[string]string{
				"kubernetes.io/service-name": svc.service,
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{
				Name: ptr.To("http"),
				Port: ptr.To(int32(8080)),
			},
			{
				Name: ptr.To("https"),
				Port: ptr.To(int32(8043)),
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.1.1.1"},
				TargetRef: &v1.ObjectReference{
					Name: "tsuru-api-0",
				},
			},
			{
				Addresses: []string{"10.1.1.2"},
				TargetRef: &v1.ObjectReference{
					Name: "tsuru-api-1",
				},
			},
		},
	}, metav1.CreateOptions{})

	instances, err := svc.LiveInstances(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.HasLen, 2)
	c.Assert(instances[0].Name, check.Equals, "tsuru-api-0")
	c.Assert(instances[1].Name, check.Equals, "tsuru-api-1")

	c.Assert(instances[0].Addresses[0], check.Equals, "10.1.1.1")
	c.Assert(instances[1].Addresses[0], check.Equals, "10.1.1.2")

	instance, err := svc.CurrentInstance(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Not(check.Equals), "")
	c.Assert(len(instance.Addresses) > 0, check.Equals, true)
}
