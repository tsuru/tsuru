// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	appTypes "github.com/tsuru/tsuru/types/app"
	apptypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestDeployV2_WithContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.p.DeployV2(ctx, nil, provision.DeployV2Args{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "context canceled")
}

func (s *S) TestDeployV2_MissingApp(c *check.C) {
	_, err := s.p.DeployV2(context.TODO(), nil, provision.DeployV2Args{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "app not provided")
}

func (s *S) TestDeployV2_MissingEvent(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	_, err := s.p.DeployV2(context.TODO(), a, provision.DeployV2Args{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event not provided")
}

func (s *S) TestDeployV2_WhenBuilderReturnsError(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	builder.Register("kubernetes", &fakeBuilder{
		OnBuildV2: func(ctx context.Context, app provision.App, evt *event.Event, opts builder.BuildOpts) (appTypes.AppVersion, error) {
			return nil, errors.New("some error")
		},
	})

	_, err = s.p.DeployV2(context.TODO(), a, provision.DeployV2Args{Event: evt})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "some error")
}

func (s *S) TestDeployV2_SuccessfulDeploy(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	builder.Register("kubernetes", &fakeBuilder{
		OnBuildV2: func(ctx context.Context, app provision.App, e *event.Event, opts builder.BuildOpts) (appTypes.AppVersion, error) {
			c.Check(ctx, check.NotNil)
			c.Check(app, check.NotNil)
			c.Check(evt, check.DeepEquals, e)

			fmt.Fprintln(opts.Output, "Running BuildV2")

			version := newCommittedVersion(c, app, map[string]any{
				"healthcheck": map[string]any{
					"path":   "/healthz",
					"status": 204,
				},
				"processes": map[string]any{
					"web":    "./path/to/server.sh --port ${PORT}",
					"worker": "./path/to/worker.sh --verbose",
				},
			})

			return version, nil
		},
	})

	var output bytes.Buffer

	image, err := s.p.DeployV2(context.TODO(), a, provision.DeployV2Args{
		Event:  evt,
		Output: &output,
	})
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, "tsuru/app-myapp:v1")

	c.Assert(output.String(), check.Matches, "(?s)(.*)Running BuildV2(.*)")

	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)

	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(appList.Items, check.HasLen, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName:        ns,
		ServiceAccountName:   "app-myapp",
		Deployments:          map[string][]string{"web": {"myapp-web"}, "worker": {"myapp-worker"}},
		Services:             map[string][]string{"web": {"myapp-web", "myapp-web-units"}, "worker": {"myapp-worker", "myapp-worker-units"}},
		PodDisruptionBudgets: map[string][]string{"web": {"myapp-web"}, "worker": {"myapp-worker"}},
	})

	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 2)

	sort.Slice(deps.Items, func(i, j int) bool { return deps.Items[i].Name < deps.Items[j].Name })

	webDeployment := deps.Items[0]
	c.Assert(webDeployment.Name, check.Equals, "myapp-web")
	c.Assert(webDeployment.Spec.Template.Spec.Containers, check.HasLen, 1)
	c.Assert(webDeployment.Spec.Template.Spec.Containers[0].Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; curl -sSL -m15 -XPOST -d\"hostname=$(hostname)\" -o/dev/null -H\"Content-Type:application/x-www-form-urlencoded\" -H\"Authorization:bearer \" http://apps/myapp/units/register || true && exec ./path/to/server.sh --port ${PORT}",
	})

	workerDeployment := deps.Items[1]
	c.Assert(workerDeployment.Name, check.Equals, "myapp-worker")
	c.Assert(workerDeployment.Spec.Template.Spec.Containers, check.HasLen, 1)
	c.Assert(workerDeployment.Spec.Template.Spec.Containers[0].Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; curl -sSL -m15 -XPOST -d\"hostname=$(hostname)\" -o/dev/null -H\"Content-Type:application/x-www-form-urlencoded\" -H\"Authorization:bearer \" http://apps/myapp/units/register || true && exec ./path/to/worker.sh --verbose",
	})

	svcs, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(svcs.Items, check.HasLen, 4)

	sort.Slice(svcs.Items, func(i, j int) bool { return svcs.Items[i].Name < svcs.Items[j].Name })

	webSvc := svcs.Items[0]
	c.Assert(webSvc.Name, check.DeepEquals, "myapp-web")

	webUnitsSvc := svcs.Items[1]
	c.Assert(webUnitsSvc.Name, check.Equals, "myapp-web-units")

	workerSvc := svcs.Items[2]
	c.Assert(workerSvc.Name, check.Equals, "myapp-worker")

	workerUnitsSvc := svcs.Items[3]
	c.Assert(workerUnitsSvc.Name, check.Equals, "myapp-worker-units")
}

type fakeBuilder struct {
	OnBuildV2 func(ctx context.Context, app provision.App, evt *event.Event, opts builder.BuildOpts) (appTypes.AppVersion, error)
}

func (f *fakeBuilder) Build(ctx context.Context, p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *builder.BuildOpts) (apptypes.AppVersion, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeBuilder) BuildV2(ctx context.Context, app provision.App, evt *event.Event, opts builder.BuildOpts) (apptypes.AppVersion, error) {
	if f.OnBuildV2 == nil {
		return nil, errors.New("not implemented")
	}

	return f.OnBuildV2(ctx, app, evt, opts)
}
