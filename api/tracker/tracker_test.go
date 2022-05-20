// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracker

import (
	"context"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types/tracker"
	check "gopkg.in/check.v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "api_tracker_pkg_tests")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
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

	cli.CoreV1().Endpoints(svc.ns).Create(context.TODO(), &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name: svc.service,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP: "10.1.1.1",
						TargetRef: &v1.ObjectReference{
							Name: "tsuru-api-0",
						},
					},
					{
						IP: "10.1.1.2",
						TargetRef: &v1.ObjectReference{
							Name: "tsuru-api-1",
						},
					},
				},
				Ports: []v1.EndpointPort{
					{
						Name: "http",
						Port: 8080,
					},
					{
						Name: "https",
						Port: 8043,
					},
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
